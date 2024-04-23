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

// Package util defines the common utils functions for cli.
package util

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path"

	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/tools"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	// UpdateFileConcurrentLimit is the limit of concurrent for update file.
	UpdateFileConcurrentLimit = 5
)

// UpdateFiles updates the files to the target directory.
func UpdateFiles(filesDir string, files []*client.ConfigItemFile) error {
	// 随机打乱配置文件顺序，避免同时下载导致的并发问题
	rand.Shuffle(len(files), func(i, j int) {
		files[i], files[j] = files[j], files[i]
	})
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(UpdateFileConcurrentLimit)
	for _, f := range files {
		file := f
		g.Go(func() error {
			// 1. prapare file path
			fileDir := path.Join(filesDir, file.Path)
			filePath := path.Join(fileDir, file.Name)
			err := os.MkdirAll(fileDir, os.ModePerm)
			if err != nil {
				return fmt.Errorf("create dir %s failed, err: %s", fileDir, err.Error())
			}
			// 2. check and download file
			exists, err := CheckFileExists(fileDir, file.FileMeta)
			if err != nil {
				return fmt.Errorf("check file exists failed, err: %s", err.Error())
			}
			if !exists {
				err := file.SaveToFile(filePath)
				if err != nil {
					return fmt.Errorf("download file failed, err: %s", err.Error())
				}
			} else {
				logger.Info("file is already exists and has not been modified, skip download", slog.String("file", filePath))
			}
			// 3. set file permission
			if err := util.SetFilePermission(filePath, file.FileMeta.ConfigItemSpec.Permission); err != nil {
				logger.Warn("set file permission failed", slog.String("file", filePath), logger.ErrAttr(err))
			}
			return nil
		})
	}
	return g.Wait()
}

// CheckFileExists checks the file exists and the SHA256 is match.
func CheckFileExists(absPath string, ci *sfs.ConfigItemMetaV1) (bool, error) {
	filePath := path.Join(absPath, ci.ConfigItemSpec.Name)
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
		logger.Info("configuration item's SHA256 is not match, need to update",
			slog.String("localHash", sha), slog.String("remoteHash", ci.ContentSpec.Signature))
		return false, nil
	}

	return true, nil
}
