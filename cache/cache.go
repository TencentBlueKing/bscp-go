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
	"os"
	"path"

	sfs "github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/tools"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/downloader"
	"github.com/TencentBlueKing/bscp-go/logger"
)

var defaultCachePath = "/tmp/bk-bscp"
var instance *Cache

// Enable define whether to enable local cache
var Enable bool

// Cache is the bscp sdk cache
type Cache struct {
	path string
}

// Init return a bscp sdk cache instance
func Init(enable bool, path string) {
	// TODO: confirm should we support overwrite the cache instance
	if path == "" {
		path = defaultCachePath
	}
	Enable = enable
	instance = &Cache{
		path: path,
	}
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
		filePath := path.Join(c.path, ci.ContentSpec.Signature)
		if err := downloader.GetDownloader().Download(ci.PbFileMeta(), ci.RepositoryPath, ci.ContentSpec.ByteSize,
			downloader.DownloadToFile, nil, filePath); err != nil {
			logger.Error("download file failed", logger.ErrAttr(err), slog.String("rid", event.Rid))
			return
		}
	}
}

// checkFileCacheExists verify the config content is exist or not in the local.
func (c *Cache) checkFileCacheExists(ci *sfs.ConfigItemMetaV1) (bool, error) {
	filePath := path.Join(c.path, ci.ContentSpec.Signature)
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
	filePath := path.Join(c.path, ci.ContentSpec.Signature)
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("read config item cache file failed",
			slog.String("file", filePath), logger.ErrAttr(err))
		return false, nil
	}
	return true, bytes
}

// CopyToFile copy the config content to the specified file.
func (c *Cache) CopyToFile(ci *sfs.ConfigItemMetaV1, filePath string) bool {
	exists, err := c.checkFileCacheExists(ci)
	if err != nil {
		logger.Warn("check config item cache exists failed",
			slog.String("item", ci.ContentSpec.Signature), logger.ErrAttr(err))
		return false
	}
	if !exists {
		return false
	}
	cacheFilePath := path.Join(c.path, ci.ContentSpec.Signature)
	src, err := os.Open(cacheFilePath)
	if err != nil {
		logger.Error("open config item cache file failed", slog.String("file", cacheFilePath), logger.ErrAttr(err))
		return false
	}
	defer src.Close()
	dst, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
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

// TODO: add cache clean logic
