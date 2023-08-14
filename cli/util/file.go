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
 *
 */

package util

import (
	"context"
	"fmt"
	"os"
	"path"

	"bscp.io/pkg/logs"
	sfs "bscp.io/pkg/sf-share"
	"bscp.io/pkg/tools"
	"golang.org/x/sync/errgroup"

	"github.com/TencentBlueKing/bscp-go/downloader"
	"github.com/TencentBlueKing/bscp-go/pkg/util"
	"github.com/TencentBlueKing/bscp-go/types"
)

const (
	// UpdateFileConcurrentLimit is the limit of concurrent for update file.
	UpdateFileConcurrentLimit = 5
)

// UpdateFiles updates the files to the target directory.
func UpdateFiles(filesDir string, files []*types.ConfigItemFile) error {
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
				err := downloader.GetDownloader().Download(file.FileMeta.PbFileMeta(), file.FileMeta.RepositoryPath,
					file.FileMeta.ContentSpec.ByteSize, downloader.DownloadToFile, nil, filePath)
				if err != nil {
					return fmt.Errorf("download file failed, err: %s", err.Error())
				}
			} else {
				logs.Infof("file %s is already exists and has not been modified, skip download", filePath)
			}
			// 3. set file permission
			if err := util.SetFilePermission(filePath, file.FileMeta.ConfigItemSpec.Permission); err != nil {
				return fmt.Errorf("set file permission for %s failed, err: %s", filePath, err.Error())
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
		logs.Infof("configuration item's SHA256 is not match, local: %s, remote: %s, need to update",
			sha, ci.ContentSpec.Signature)
		return false, nil
	}

	return true, nil
}
