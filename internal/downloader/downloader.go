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
	"path"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	pbfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/feed-server"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"golang.org/x/sync/semaphore"

	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	instance *downloader

	// DownloadToBytes download file content to bytes.
	DownloadToBytes DownloadTo = "bytes"
	// DownloadToFile download file content to file.
	DownloadToFile DownloadTo = "file"
)

// DownloadTo defines the download target.
type DownloadTo string

// Downloader implements all the supported operations which used to download files from provider.
// default max memory usage: defaultDownloadGroutines(default 10) * swapSize(default 2MB) = 20MB
type Downloader interface {
	// Download the configuration items from provider.
	// path is the full path of the file to be downloaded.
	Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64, to DownloadTo, b []byte, path string) error
}

// Init init the downloader instance.
func Init(vas *kit.Vas, bizID uint32, token string, upstream upstream.Upstream, tlsBytes *sfs.TLSBytes,
	enableP2P bool, agentID, clusterID, podID, containerName string) error {

	tlsC, err := tlsConfigFromTLSBytes(tlsBytes)
	if err != nil {
		return fmt.Errorf("build tls config failed, err: %s", err.Error())
	}

	instance = &downloader{
		httpDownloader: &httpDownloader{
			vas:                     vas,
			token:                   token,
			bizID:                   bizID,
			upstream:                upstream,
			tls:                     tlsC,
			sem:                     semaphore.NewWeighted(setupMaxHttpDownloadGoroutines()),
			balanceDownloadByteSize: defaultRangeDownloadByteSize,
		},
	}

	if !enableP2P {
		logger.Warn("async p2p download is set to disabled")
		return nil
	}
	if agentID == "" && (clusterID == "" || podID == "" || containerName == "") {
		logger.Warn("async download is disabled, because agentID or clusterID/podID/containerName is empty")
		return nil
	}
	instance.enableAsyncDownload = true
	instance.asyncDownloader = &asyncDownloader{
		vas:           vas,
		token:         token,
		bizID:         bizID,
		upstream:      upstream,
		bkAgentID:     agentID,
		clusterID:     clusterID,
		podID:         podID,
		containerName: containerName,
	}

	return nil
}

type downloader struct {
	enableAsyncDownload bool
	asyncDownloader     *asyncDownloader
	httpDownloader      *httpDownloader
}

func (d *downloader) Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64, to DownloadTo, b []byte,
	filePath string) error {
	if d.enableAsyncDownload && to == DownloadToFile {
		if err := d.asyncDownloader.Download(fileMeta, downloadUri, fileSize, to, b, filePath); err != nil {
			logger.Warn("async download file failed, fallback to http download", "file",
				path.Join(fileMeta.ConfigItemSpec.Path, fileMeta.ConfigItemSpec.Name), "err", err.Error())
			return d.httpDownloader.Download(fileMeta, downloadUri, fileSize, to, b, filePath)
		}
	}
	return d.httpDownloader.Download(fileMeta, downloadUri, fileSize, to, b, filePath)
}
