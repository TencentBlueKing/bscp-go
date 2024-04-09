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

// Package downloader defines the config item downloader.
package downloader

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	pbfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/feed-server"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/tools"

	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

type asyncDownloader struct {
	vas *kit.Vas
	// bkAgentID blueking gse agent id
	bkAgentID string
	// clusterID bcs cluster id
	clusterID string
	// podID k8s pod id
	podID string
	// containerName bscp container name in pod
	containerName string
	upstream      upstream.Upstream
	bizID         uint32
	token         string
}

// Download the configuration items from provider.
func (dl *asyncDownloader) Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64,
	to DownloadTo, bytes []byte, toFile string) error {
	task, err := dl.upstream.AsyncDownload(dl.vas, &pbfs.AsyncDownloadReq{
		BizId:         fileMeta.ConfigItemAttachment.BizId,
		BkAgentId:     dl.bkAgentID,
		ClusterId:     dl.clusterID,
		PodId:         dl.podID,
		ContainerName: dl.containerName,
		FileMeta:      fileMeta,
		FileDir:       filepath.Dir(toFile),
	})
	if err != nil {
		return err
	}
	// TODO: set time out
	timeoutVas, cancel := dl.vas.WithTimeout(10 * time.Minute)
	defer cancel()
	type downloadStatus struct {
		status pbfs.AsyncDownloadStatus
		err    error
	}
	resultChan := make(chan downloadStatus)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-timeoutVas.Ctx.Done():
				resultChan <- downloadStatus{
					status: pbfs.AsyncDownloadStatus_FAILED,
					err:    timeoutVas.Ctx.Err(),
				}
				return
			case <-ticker.C:
				resp, e := dl.upstream.AsyncDownloadStatus(dl.vas, &pbfs.AsyncDownloadStatusReq{
					BizId:  fileMeta.ConfigItemAttachment.BizId,
					TaskId: task.TaskId,
				})
				if e != nil {
					resultChan <- downloadStatus{
						status: pbfs.AsyncDownloadStatus_FAILED,
						err:    e,
					}
					return
				}
				switch resp.Status {
				case pbfs.AsyncDownloadStatus_SUCCESS:
					resultChan <- downloadStatus{
						status: pbfs.AsyncDownloadStatus_SUCCESS,
						err:    nil,
					}
					return
				case pbfs.AsyncDownloadStatus_FAILED:
					resultChan <- downloadStatus{
						status: pbfs.AsyncDownloadStatus_FAILED,
						err:    nil,
					}
					return
				case pbfs.AsyncDownloadStatus_DOWNLOADING:
					continue
				}
			}
		}
	}()
	result := <-resultChan
	if result.err != nil {
		return result.err
	}

	switch result.status {
	case pbfs.AsyncDownloadStatus_SUCCESS:
	case pbfs.AsyncDownloadStatus_FAILED:
		return fmt.Errorf("async download file %s failed, err: %v", toFile, result.err)
	}

	sign, err := tools.FileSHA256(toFile)
	if err != nil {
		return fmt.Errorf("check file %s sha256 failed, err: %s", toFile, err.Error())
	}

	if sign != fileMeta.CommitSpec.Content.Signature {
		return fmt.Errorf("file %s sha256 not matched, file sha256: %s, meta sha256: %s",
			toFile, sign, fileMeta.CommitSpec.Content.Signature)
	}

	logger.Info("async download file %s success", toFile)

	return nil
}
