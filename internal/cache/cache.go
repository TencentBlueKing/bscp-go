/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package cache defines the config item cache.
package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/tools"
	"github.com/dustin/go-humanize"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/internal/downloader"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	// GByte is gigabyte unit
	GByte = 1024 * 1024 * 1024

	// MaxSingleFileCacheSizeRate max size rate of single file cache
	// file size bigger than this rate will not be cached
	MaxSingleFileCacheSizeRate = 0.1
)

var instance *Cache

// Enable define whether to enable local cache
var Enable bool

// Cache is the bscp sdk cache
type Cache struct {
	path       string
	thrsholdGB float64
}

// Init return a bscp sdk cache instance
func Init(path string, thresholdGB float64) error {
	Enable = true
	instance = &Cache{
		path:       path,
		thrsholdGB: thresholdGB,
	}

	// prepare cache dir
	return os.MkdirAll(path, os.ModePerm)
}

// GetCache return the cache instance
func GetCache() *Cache {
	return instance
}

// OnReleaseChange is the callback to refresh cache when release change event was received.
func (c *Cache) OnReleaseChange(event *sfs.ReleaseChangeEvent) {
	pl := new(sfs.ReleaseChangePayload)
	if err := json.Unmarshal(event.Payload, pl); err != nil {
		logger.Error("decode release change event payload failed, skip the event",
			logger.ErrAttr(err), slog.String("rid", event.Rid))
		return
	}

	if err := os.MkdirAll(c.path, os.ModePerm); err != nil {
		logger.Error("mkdir cache path failed", slog.String("path", c.path), logger.ErrAttr(err))
		return
	}

	for _, ci := range pl.ReleaseMeta.CIMetas {
		exists, err := c.checkFileCacheExists(ci)
		if err != nil {
			logger.Error("check config item exists failed", logger.ErrAttr(err), slog.String("rid", event.Rid))
			continue
		}
		if exists {
			continue
		}
		filePath := filepath.Join(c.path, ci.ContentSpec.Signature)
		// TODO: gse 现在分发文件时，target 的目录必须一致，因此这里 Cache 和 SDK 的下载目录会被视为同一个目录，并发下载时会有问题
		// 两个并发下载任务下载到同一个文件中，但是 Downloader 中并发移动这个文件时会导致其中一个任务失败
		// 在 GSE 解决这个问题（支持根据 target 设置目录）之前，先不启用 Cahce.OnReleaseChange 回调
		if err := downloader.GetDownloader().Download(ci.PbFileMeta(), ci.RepositoryPath, ci.ContentSpec.ByteSize,
			downloader.DownloadToFile, nil, filePath); err != nil {
			logger.Error("download file failed", logger.ErrAttr(err), slog.String("rid", event.Rid))
			return
		}
	}
}

// checkFileCacheExists verify the config content is exist or not in the local.
func (c *Cache) checkFileCacheExists(ci *sfs.ConfigItemMetaV1) (bool, error) {
	filePath := filepath.Join(c.path, ci.ContentSpec.Signature)
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// content is not exist
			return false, nil
		}

		return false, err
	}

	sha, err := tools.FileSHA256(filePath)
	if err != nil {
		return false, fmt.Errorf("check configuration item's SHA256 failed, err: %s", err.Error())
	}

	if sha != ci.ContentSpec.Signature {
		return false, nil
	}

	return true, nil
}

// GetFileContent return the config content bytes.
func (c *Cache) GetFileContent(ci *sfs.ConfigItemMetaV1) (bool, []byte) {
	exists, err := c.checkFileCacheExists(ci)
	if err != nil {
		logger.Error("check config item cache exists failed",
			slog.String("item", ci.ContentSpec.Signature), logger.ErrAttr(err))
		return false, nil
	}
	if !exists {
		return false, nil
	}
	filePath := filepath.Join(c.path, ci.ContentSpec.Signature)
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("read config item cache file failed",
			slog.String("file", filePath), logger.ErrAttr(err))
		return false, nil
	}
	return true, bytes
}

// CopyToFile copy the config content to the specified file.
// get from cache first, if not exist, then get from remote repo and add it to cache
func (c *Cache) CopyToFile(ci *sfs.ConfigItemMetaV1, filePath string) bool {
	if ci.ContentSpec.ByteSize > uint64(MaxSingleFileCacheSizeRate*c.thrsholdGB*GByte) {
		logger.Warn("config item size is too large, skip cache",
			slog.String("item", filepath.Join(ci.ConfigItemSpec.Path, ci.ConfigItemSpec.Name)),
			slog.Int64("size", int64(ci.ContentSpec.ByteSize)))
		return false
	}
	exists, err := c.checkFileCacheExists(ci)
	if err != nil {
		logger.Error("check config item cache exists failed",
			slog.String("item", ci.ContentSpec.Signature), logger.ErrAttr(err))
		return false
	}

	cacheFilePath := filepath.Join(c.path, ci.ContentSpec.Signature)
	if !exists {
		// get from remote repo and add it to cache
		if err = downloader.GetDownloader().Download(ci.PbFileMeta(), ci.RepositoryPath, ci.ContentSpec.ByteSize,
			downloader.DownloadToFile, nil, cacheFilePath); err != nil {
			logger.Error("download file failed", logger.ErrAttr(err))
			return false
		}
	}

	var src, dst *os.File
	src, err = os.Open(cacheFilePath)
	if err != nil {
		logger.Error("open config item cache file failed", slog.String("file", cacheFilePath), logger.ErrAttr(err))
		return false
	}
	defer src.Close()

	dst, err = os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		logger.Error("open destination file failed", slog.String("file", filePath), logger.ErrAttr(err))
		return false
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		logger.Error("copy config item cache file to destination file failed",
			slog.String("cache_file", cacheFilePath), slog.String("file", filePath), logger.ErrAttr(err))
		return false
	}
	return true
}

// AutoCleanupFileCache auto cleanup file cache
func AutoCleanupFileCache(cacheDir string, cleanupIntervalSeconds int64, thresholdGB, retentionRate float64) {
	logger.Info("start auto cleanup file cache ",
		slog.String("cacheDir", cacheDir),
		slog.String("cleanupIntervalSeconds", fmt.Sprintf("%ds", cleanupIntervalSeconds)),
		slog.String("thresholdGB", fmt.Sprintf("%sGB", humanize.Ftoa(thresholdGB))),
		slog.String("retentionRate", fmt.Sprintf("%s%%", humanize.Ftoa(retentionRate*100))))

	for {
		currentSize, err := calculateDirSize(cacheDir)
		if err != nil {
			logger.Error("calculate current cache directory size failed", logger.ErrAttr(err))
			time.Sleep(time.Duration(cleanupIntervalSeconds) * time.Second)
			continue
		}
		logger.Debug("calculate current cache directory size", slog.String("currentSize",
			humanize.IBytes(uint64(currentSize))))

		if currentSize > int64(thresholdGB*GByte) {
			logger.Info("cleaning up directory...")
			cleanupOldestFiles(cacheDir, currentSize-int64(math.Floor(thresholdGB*GByte*retentionRate)))
		}
		time.Sleep(time.Duration(cleanupIntervalSeconds) * time.Second)
	}
}

func calculateDirSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func cleanupOldestFiles(dir string, spaceToFree int64) {
	files, err := listFilesByModTime(dir)
	if err != nil {
		logger.Error("list files by mod time failed", logger.ErrAttr(err))
	}

	for _, file := range files {
		filePath := filepath.Join(dir, file.Name())
		err = os.Remove(filePath)
		if err != nil {
			logger.Error("deleting file failed", slog.String("file", filePath), logger.ErrAttr(err))
		} else {
			logger.Info("deleted file", slog.String("file", filePath))
			spaceToFree -= file.Size()
		}

		if spaceToFree <= 0 {
			break
		}
	}
}

func listFilesByModTime(dir string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, info)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	return files, nil
}
