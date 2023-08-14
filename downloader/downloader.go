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

package downloader

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"bscp.io/pkg/criteria/constant"
	"bscp.io/pkg/kit"
	"bscp.io/pkg/logs"
	pbfs "bscp.io/pkg/protocol/feed-server"
	sfs "bscp.io/pkg/sf-share"
	"golang.org/x/sync/semaphore"

	"github.com/TencentBlueKing/bscp-go/upstream"
)

const (
	// TODO: consider config these options.
	defaultSwapBufferSize              = 2 * 1024 * 1024
	defaultRangeDownloadByteSize       = 5 * defaultSwapBufferSize
	requestAwaitResponseTimeoutSeconds = 10
	defaultDownloadSemaphoreWight      = 10
)

type DownloadTo string

var (
	instance *downloader

	DownloadToBytes DownloadTo = "bytes"
	DownloadToFile  DownloadTo = "file"
)

// Downloader implements all the supported operations which used to download
// files from provider.
type Downloader interface {
	// Download the configuration items from provider.
	// path is the full path of the file to be downloaded.
	Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64, to DownloadTo, b []byte, path string) error
}

// Init init the downloader instance.
func Init(vas *kit.Vas, bizID uint32, token string, upstream upstream.Upstream, tlsBytes *sfs.TLSBytes) error {

	tlsC, err := tlsConfigFromTLSBytes(tlsBytes)
	if err != nil {
		return fmt.Errorf("build tls config failed, err: %s", err.Error())
	}

	weight, err := setupDownloadSemWeight()
	if err != nil {
		return fmt.Errorf("get download sem weight failed, err: %s", err.Error())
	}

	instance = &downloader{
		vas:                     vas,
		token:                   token,
		bizID:                   bizID,
		upstream:                upstream,
		tls:                     tlsC,
		sem:                     semaphore.NewWeighted(weight),
		balanceDownloadByteSize: defaultRangeDownloadByteSize,
	}
	return nil
}

// setupDownloadSemWeight maximum combined weight for concurrent download access.
func setupDownloadSemWeight() (int64, error) {
	weightEnv := os.Getenv(constant.EnvMaxDownloadFileGoroutines)
	if len(weightEnv) == 0 {
		return defaultDownloadSemaphoreWight, nil
	}

	weight, err := strconv.ParseInt(weightEnv, 10, 64)
	if err != nil {
		return 0, err
	}

	if weight < 1 {
		return 0, errors.New("invalid download sem weight, should >= 1")
	}

	if weight > 15 {
		return 0, errors.New("invalid download sem weight, should <= 15")
	}

	return weight, nil
}

// GetDownloader returns the downloader instance.
func GetDownloader() Downloader {
	return instance
}

// downloader is used to download the configuration items from provider
type downloader struct {
	vas      *kit.Vas
	upstream upstream.Upstream
	bizID    uint32
	token    string
	tls      *tls.Config
	sem      *semaphore.Weighted
	// balanceDownloadByteSize determines when to download the file with range policy
	// if the configuration item's content size is larger than this, then it
	// will be downloaded with range policy, otherwise, it will be downloaded directly
	// without range policy.
	balanceDownloadByteSize uint64
}

// Download the configuration items from provider.
func (dl *downloader) Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64,
	to DownloadTo, bytes []byte, toFile string) error {
	exec := &execDownload{
		ctx:         context.Background(),
		dl:          dl,
		fileMeta:    fileMeta,
		to:          to,
		client:      dl.initClient(),
		header:      http.Header{},
		downloadUri: downloadUri,
		fileSize:    fileSize,
	}
	switch to {
	case DownloadToFile:
		if len(toFile) == 0 {
			return fmt.Errorf("target file path is empty")
		}
		file, err := os.OpenFile(toFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
		if err != nil {
			return fmt.Errorf("open the target file failed, err: %s", err.Error())
		}
		defer file.Close()
		exec.file = file
	case DownloadToBytes:
		if len(bytes) != int(fileSize) {
			return fmt.Errorf("the size of bytes is not equal to the file size")
		}
		exec.bytes = bytes
	}
	return exec.do()
}

func (dl *downloader) initClient() *http.Client {
	// TODO: find a way to manage these configuration options.
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig:     dl.tls,
		Dial: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		MaxIdleConnsPerHost: 10,
		// TODO: confirm this
		ResponseHeaderTimeout: 15 * time.Minute,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   0,
	}
}

type execDownload struct {
	fileMeta    *pbfs.FileMeta
	ctx         context.Context
	dl          *downloader
	to          DownloadTo
	bytes       []byte
	file        *os.File
	client      *http.Client
	header      http.Header
	downloadUri string
	fileSize    uint64
}

func (exec *execDownload) do() error {
	// get file temporary download url from upstream
	getUrlReq := &pbfs.GetDownloadURLReq{
		ApiVersion: sfs.CurrentAPIVersion,
		BizId:      exec.dl.bizID,
		FileMeta:   exec.fileMeta,
		Token:      exec.dl.token,
	}
	resp, err := exec.dl.upstream.GetDownloadURL(exec.dl.vas, getUrlReq)
	if err != nil {
		return fmt.Errorf("get temporary download url failed, err: %s", err.Error())
	}
	exec.downloadUri = resp.Url
	if exec.fileSize <= exec.dl.balanceDownloadByteSize {
		// the file size is not big enough, download directly
		if err := exec.downloadDirectly(requestAwaitResponseTimeoutSeconds); err != nil {
			return fmt.Errorf("download directly failed, err: %s", err.Error())
		}

		return nil
	}

	size, yes, err := exec.isProviderSupportRangeDownload()
	if err != nil {
		logs.Warnf("check if provider support range download failed because of %s", err.Error())
	}

	if yes {
		if size != exec.fileSize {
			return fmt.Errorf("the to be download file size: %d is not as what we expected %d", size, exec.fileSize)
		}

		if err := exec.downloadWithRange(); err != nil {
			return fmt.Errorf("download with range failed, err: %s", err.Error())
		}

		return nil
	}

	logs.Warnf("provider does not support download with range policy, download directly now.")

	if err := exec.downloadDirectly(requestAwaitResponseTimeoutSeconds); err != nil {
		return fmt.Errorf("download directly failed, err: %s", err.Error())
	}

	return nil
}

// isProviderSupportRangeDownload return the configuration item's content size if
// it supports range download.
func (exec *execDownload) isProviderSupportRangeDownload() (uint64, bool, error) {
	req, err := http.NewRequest(http.MethodHead, exec.downloadUri, nil)
	if err != nil {
		return 0, false, fmt.Errorf("new request failed, err: %s", err.Error())
	}

	req = req.WithContext(exec.ctx)
	req.Header = exec.header
	req.Header.Set("Request-Timeout", strconv.Itoa(15))

	resp, err := exec.client.Do(req)
	if err != nil {
		return 0, false, fmt.Errorf("do request failed, err: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("request to provider failed http code: %d", resp.StatusCode)
	}

	mode, exist := resp.Header["Accept-Ranges"]
	if !exist {
		return 0, false, nil
	}

	if len(mode) == 0 {
		return 0, false, nil
	}

	if mode[0] != "bytes" {
		return 0, false, nil
	}

	length, exist := resp.Header["Content-Length"]
	if !exist {
		return 0, false, errors.New("can not get the content length form header")
	}

	if len(length) == 0 {
		return 0, false, errors.New("can not get the content length form header")
	}

	size, err := strconv.ParseUint(length[0], 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse content length failed, err: %s", err.Error())
	}

	return size, true, nil

}

// downloadDirectly download file without range.
func (exec *execDownload) downloadDirectly(timeoutSeconds int) error {
	if err := exec.dl.sem.Acquire(exec.ctx, 1); err != nil {
		return fmt.Errorf("acquire semaphore failed, err: %s", err.Error())
	}

	defer exec.dl.sem.Release(1)

	start := time.Now()
	header := exec.header
	body, err := exec.doRequest(http.MethodGet, header, timeoutSeconds)
	if err != nil {
		return err
	}
	if err := exec.write(body, exec.fileSize, 0); err != nil {
		return err
	}

	logs.V(0).Infof("file[%s], download directly success, cost: %s",
		path.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name), time.Since(start).String())

	return nil
}

func (exec *execDownload) downloadWithRange() error {

	logs.Infof("start download file[%s] with range",
		path.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name))

	var start, end uint64
	batchSize := 2 * exec.dl.balanceDownloadByteSize
	// calculate the total parts to be downloaded
	totalParts := int(exec.fileSize / batchSize)
	if (exec.fileSize % batchSize) > 0 {
		totalParts += 1
	}

	var hitError error
	wg := sync.WaitGroup{}

	for part := 0; part < totalParts; part++ {
		if err := exec.dl.sem.Acquire(exec.ctx, 1); err != nil {
			return fmt.Errorf("acquire semaphore failed, err: %s", err.Error())
		}

		start = uint64(part) * batchSize

		if part == totalParts-1 {
			end = exec.fileSize
		} else {
			end = start + batchSize
		}

		end -= 1

		wg.Add(1)

		go func(pos int, from uint64, to uint64) {
			defer func() {
				wg.Done()
				exec.dl.sem.Release(1)
			}()

			start := time.Now()
			if err := exec.downloadOneRangedPart(from, to); err != nil {
				hitError = err
				logs.Errorf("download file[%s] part %d failed, start: %d, err: %s",
					path.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name),
					pos, from, err.Error())
				return
			}

			logs.V(0).Infof("download file range part %d success, range [%d, %d], cost: %s", pos, from, to,
				time.Since(start).String())

		}(part, start, end)

	}

	wg.Wait()

	if hitError != nil {
		return hitError
	}

	logs.V(1).Infof("download full file[%s] success",
		path.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name))

	return nil
}

func (exec *execDownload) downloadOneRangedPart(start uint64, end uint64) error {
	if start > end {
		return errors.New("invalid start or end to do range download")
	}

	header := exec.header.Clone()
	// set ranged part.
	if start == end {
		header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	} else {
		header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	}

	body, err := exec.doRequest(http.MethodGet, header, 6*requestAwaitResponseTimeoutSeconds)
	if err != nil {
		return err
	}

	defer body.Close()

	if err := exec.write(body, end-start+1, start); err != nil {
		return err
	}

	return nil
}

func (exec *execDownload) doRequest(method string, header http.Header, timeoutSeconds int) (io.ReadCloser, error) {
	req, err := http.NewRequest(method, exec.downloadUri, nil)
	if err != nil {
		return nil, fmt.Errorf("new request failed, err: %s", err.Error())
	}

	req.Header = header
	// Note: do not use request context to control timeout, the context is
	// managed by the upper scheduler.
	if timeoutSeconds > 0 {
		req.Header.Set("Request-Timeout", strconv.Itoa(timeoutSeconds))
	}

	req = req.WithContext(exec.ctx)

	resp, err := exec.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request failed, err: %s", err.Error())
	}

	// download with range's http status code is 206:StatusPartialContent
	// reference:
	// https://developer.mozilla.org/zh-CN/docs/Web/HTTP/Range_requests
	// https://datatracker.ietf.org/doc/html/rfc7233#page-8
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("request to provider, but returned with http code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (exec *execDownload) write(body io.ReadCloser, expectSize uint64, start uint64) error {
	totalSize := uint64(0)
	swap := make([]byte, defaultSwapBufferSize)
	for {
		select {
		case <-exec.ctx.Done():
			return fmt.Errorf("download context done, %s", exec.ctx.Err())

		default:
		}

		picked, err := body.Read(swap)
		// we should always process the n > 0 bytes returned before
		// considering the error err. Doing so correctly handles I/O errors
		// that happen after reading some bytes and also both of the
		// allowed EOF behaviors.
		if picked > 0 {
			var cnt int
			switch exec.to {
			case DownloadToBytes:
				for i := 0; i < picked; i++ {
					exec.bytes[totalSize+uint64(i)] = swap[i]
				}
			case DownloadToFile:
				cnt, err = exec.file.WriteAt(swap[0:picked], int64(start+totalSize))
				if err != nil {
					return fmt.Errorf("write data to file failed, err: %s", err.Error())
				}
				if cnt != picked {
					return fmt.Errorf("writed to file's size: %d is not as what we expected: %d", cnt, picked)
				}
			}

			totalSize += uint64(picked)
		}

		if err == nil {
			continue
		}

		if err != io.EOF {
			return fmt.Errorf("read data from response body failed, err: %s", err.Error())
		}

		break
	}

	// the file has already been downloaded to the local, check the file size now.
	if totalSize != expectSize {
		return fmt.Errorf("the downloaded file's total size %d is not what we expected %d", totalSize, expectSize)
	}

	return nil
}

func tlsConfigFromTLSBytes(tlsBytes *sfs.TLSBytes) (*tls.Config, error) {
	if tlsBytes == nil {
		return new(tls.Config), nil
	}

	var caPool *x509.CertPool
	if len(tlsBytes.CaFileBytes) != 0 {
		caPool = x509.NewCertPool()
		if !caPool.AppendCertsFromPEM([]byte(tlsBytes.CaFileBytes)) {
			return nil, fmt.Errorf("append ca cert failed")
		}
	}

	var certificate tls.Certificate
	if len(tlsBytes.CertFileBytes) == 0 && len(tlsBytes.CertFileBytes) == 0 { //nolint:staticcheck
		return &tls.Config{
			InsecureSkipVerify: tlsBytes.InsecureSkipVerify,
			ClientCAs:          caPool,
			Certificates:       []tls.Certificate{certificate},
			ClientAuth:         tls.RequireAndVerifyClientCert,
		}, nil
	}

	tlsCert, err := tls.X509KeyPair([]byte(tlsBytes.CertFileBytes), []byte(tlsBytes.KeyFileBytes))
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		InsecureSkipVerify: tlsBytes.InsecureSkipVerify,
		ClientCAs:          caPool,
		Certificates:       []tls.Certificate{tlsCert},
		ClientAuth:         tls.RequireAndVerifyClientCert,
	}, nil
}
