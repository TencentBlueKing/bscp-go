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

package client

import (
	"fmt"

	pbci "github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/config-item"
	pbhook "github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/hook"
	sfs "github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/internal/cache"
	"github.com/TencentBlueKing/bscp-go/internal/downloader"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

// Release bscp 服务版本
type Release struct {
	ReleaseID uint32            `json:"release_id"`
	FileItems []*ConfigItemFile `json:"files"`
	KvItems   []*sfs.KvMetaV1   `json:"kvs"`
	PreHook   *pbhook.HookSpec  `json:"pre_hook"`
	PostHook  *pbhook.HookSpec  `json:"post_hook"`
}

// ConfigItemFile defines config item file
type ConfigItemFile struct {
	// Config file name
	Name string `json:"name"`
	// Path of config file
	Path string `json:"path"`
	// Permission file permission
	Permission *pbci.FilePermission `json:"permission"`
	// FileMeta data
	FileMeta *sfs.ConfigItemMetaV1 `json:"fileMeta"`
}

// GetContent Get file binary content from cache or download from remote
func (c *ConfigItemFile) GetContent() ([]byte, error) {
	if cache.Enable {
		if hit, bytes := cache.GetCache().GetFileContent(c.FileMeta); hit {
			return bytes, nil
		}
	}
	bytes := make([]byte, c.FileMeta.ContentSpec.ByteSize)

	if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
		c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToBytes, bytes, ""); err != nil {
		return nil, fmt.Errorf("download file failed, err: %s", err.Error())
	}
	return bytes, nil
}

// SaveToFile save file content and write to local file
func (c *ConfigItemFile) SaveToFile(src string) error {
	// 1. check if cache hit, copy from cache
	if cache.Enable && cache.GetCache().CopyToFile(c.FileMeta, src) {
		logger.Info("copy file from cache success", slog.String("src", src))
	} else {
		// 2. if cache not hit, download file from remote
		if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
			c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToFile, nil, src); err != nil {
			return fmt.Errorf("download file failed, err %s", err.Error())
		}
	}
	// 3. set file permission
	if err := util.SetFilePermission(src, c.Permission); err != nil {
		logger.Warn("set file permission failed", slog.String("file", src), logger.ErrAttr(err))
	}

	return nil
}

// Callback watch callback
type Callback func(release *Release) error
