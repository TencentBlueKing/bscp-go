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
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	pbfs "github.com/TencentBlueKing/bk-bscp/pkg/protocol/feed-server"
	"github.com/TencentBlueKing/bk-bscp/pkg/tools"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	defaultAsyncDownloadByteSize             = 2 * 1024 * 1024
	defaultAsyncDownloadPollingStateInterval = 5 * time.Second
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

// Download the configuration items from p2p async download.
func (dl *asyncDownloader) Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64,
	to DownloadTo, bytes []byte, toFile string) error {
	// create asynchronous download task

	start := time.Now()

	tempDir := os.TempDir()
	resp, err := dl.upstream.AsyncDownload(dl.vas, &pbfs.AsyncDownloadReq{
		BizId:         fileMeta.ConfigItemAttachment.BizId,
		BkAgentId:     dl.bkAgentID,
		ClusterId:     dl.clusterID,
		PodId:         dl.podID,
		ContainerName: dl.containerName,
		FileMeta:      fileMeta,
		FileDir:       tempDir,
	})
	if err != nil {
		return err
	}

	logger.Info("start async download file",
		slog.String("file", filepath.Join(fileMeta.ConfigItemSpec.Path, fileMeta.ConfigItemSpec.Name)),
		slog.String("taskID", resp.TaskId))

	// Check the status of the download asynchronously with timeout
	if err := dl.awaitDownloadCompletion(fileMeta.ConfigItemAttachment.BizId, resp.TaskId, toFile); err != nil {
		return err
	}

	// move the downloaded file from temp dir to the target path
	if err := MoveFile(filepath.Join(tempDir, fileMeta.CommitSpec.Content.Signature), toFile); err != nil {
		return fmt.Errorf("move file from %s to %s failed, err: %s",
			filepath.Join(tempDir, fileMeta.CommitSpec.Content.Signature), toFile, err)
	}

	// Verify the checksum of the downloaded file
	if err := dl.verifyChecksum(toFile, fileMeta.CommitSpec.Content.Signature); err != nil {
		return err
	}

	logger.Info("async download file success", "file", toFile, "cost", time.Since(start).String())
	return nil
}

// awaitDownloadCompletion waits for the download task to complete with a timeout.
func (dl *asyncDownloader) awaitDownloadCompletion(bizID uint32, taskID, toFile string) error {
	ctx, cancel := context.WithTimeout(dl.vas.Ctx, 10*time.Minute)
	defer cancel()

	ticker := time.NewTicker(defaultAsyncDownloadPollingStateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("async download file %s timed out", toFile)
		case <-ticker.C:

			resp, err := dl.upstream.AsyncDownloadStatus(dl.vas, &pbfs.AsyncDownloadStatusReq{
				BizId:  bizID,
				TaskId: taskID,
			})
			if err != nil {
				return err
			}
			switch resp.Status {
			case pbfs.AsyncDownloadStatus_FAILED:
				return fmt.Errorf("async download file %s failed", toFile)
			case pbfs.AsyncDownloadStatus_DOWNLOADING:
				continue
			case pbfs.AsyncDownloadStatus_SUCCESS:
				return nil
			}
		}
	}
}

// verifyChecksum verifies the checksum of the downloaded file.
func (dl *asyncDownloader) verifyChecksum(filePath, expectedChecksum string) error {
	signature, err := tools.FileSHA256(filePath)
	if err != nil {
		return fmt.Errorf("check file %s SHA256 failed, err: %s", filePath, err)
	}

	if signature != expectedChecksum {
		return fmt.Errorf("file %s SHA256 not matched, file SHA256: %s, expected SHA256: %s",
			filePath, signature, expectedChecksum)
	}

	return nil
}

// MoveFile move file from srcPath to dstPath, if cross-device link error, copy file and remove original file.
func MoveFile(srcPath, dstPath string) error {
	// try move file through os.Rename
	err := os.Rename(srcPath, dstPath)
	if err != nil {
		// cross-device link error
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err == syscall.EXDEV {
			return crossDeviceMoveFile(srcPath, dstPath)
		}
		return err
	}
	return nil
}

func crossDeviceMoveFile(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	if err := dstFile.Sync(); err != nil {
		return err
	}

	if err := os.Remove(srcPath); err != nil {
		logger.Error("failed to remove original file after copy", logger.ErrAttr(err))
	}
	return nil
}
