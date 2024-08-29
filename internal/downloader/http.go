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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	pbbase "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/base"
	pbfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/feed-server"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/tools"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	// TODO: consider config these options.
	defaultSwapBufferSize              = 2 * 1024 * 1024
	defaultRangeDownloadByteSize       = 5 * defaultSwapBufferSize
	requestAwaitResponseTimeoutSeconds = 10
	defaultDownloadGroutines           = 10

	// EnvMaxHTTPDownloadGoroutines is the env name of max goroutines to download file via http.
	EnvMaxHTTPDownloadGoroutines = "BK_BSCP_MAX_HTTP_DOWNLOAD_GOROUTINES"
)

var (
	// swapPool is the swap buffer pool.
	swapPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, defaultSwapBufferSize)
			return &b
		},
	}
)

// setupMaxHttpDownloadGoroutines max goroutines to download file via http.
func setupMaxHttpDownloadGoroutines() int64 {
	weightEnv := os.Getenv(EnvMaxHTTPDownloadGoroutines)
	if len(weightEnv) == 0 {
		return defaultDownloadGroutines
	}

	weight, err := strconv.ParseInt(weightEnv, 10, 64)
	if err != nil {
		logger.Warn("invalid max http download groutines, set to default for now",
			slog.String("groutines", weightEnv),
			slog.Int("default", defaultDownloadGroutines))
		return defaultDownloadGroutines
	}

	if weight < 1 {
		logger.Warn("invalid max http download groutines, should >= 1, set to 1 for now", slog.Int64("groutines", weight))
		return 1
	}

	if weight > 15 {
		logger.Warn("invalid max http download groutines, should <= 15, set to 1 for now", slog.Int64("groutines", weight))
	}

	logger.Info("max http download groutines", slog.Int64("groutines", weight))

	return weight
}

// GetDownloader returns the downloader instance.
func GetDownloader() Downloader {
	return instance
}

// httpDownloader is used to download the configuration items from provider
type httpDownloader struct {
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
func (dl *httpDownloader) Download(fileMeta *pbfs.FileMeta, downloadUri string, fileSize uint64,
	to DownloadTo, bytes []byte, toFile string) error {

	start := time.Now()
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
			return sfs.WrapPrimaryError(sfs.DownloadFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.FilePathNotFound,
					Err: fmt.Errorf("target file path is empty")})
		}
		file, err := os.OpenFile(toFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
		if err != nil {
			return sfs.WrapPrimaryError(sfs.DownloadFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.OpenFileFailed,
					Err: fmt.Errorf("open the target file failed, err: %s", err.Error())})
		}
		logger.Info("open file success", "file", toFile)
		defer file.Close()
		exec.file = file
	case DownloadToBytes:
		if len(bytes) != int(fileSize) {
			return sfs.WrapPrimaryError(sfs.DownloadFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.ValidateDownloadFailed,
					Err: fmt.Errorf("the size of bytes is not equal to the file size")})
		}
		exec.bytes = bytes
	}
	if err := exec.do(); err != nil {
		return err
	}

	logger.Info("http download file success", "file", toFile, "cost", time.Since(start).String())
	return nil
}

func (dl *httpDownloader) initClient() *http.Client {
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
	dl          *httpDownloader
	to          DownloadTo
	bytes       []byte
	file        *os.File
	client      *http.Client
	header      http.Header
	downloadUri string
	fileSize    uint64
	waitTimeMil int64
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
		if st, ok := status.FromError(err); ok {
			if st.Code() == codes.PermissionDenied || st.Code() == codes.Unauthenticated {
				return sfs.WrapPrimaryError(sfs.TokenFailed,
					sfs.SecondaryError{SpecificFailedReason: sfs.TokenPermissionFailed,
						Err: st.Err()})
			}

			if st.Code() == codes.FailedPrecondition {
				for _, detail := range st.Details() {
					if d, ok := detail.(*pbbase.ErrDetails); ok {
						return sfs.WrapPrimaryError(sfs.FailedReason(d.PrimaryError),
							sfs.SecondaryError{SpecificFailedReason: sfs.SpecificFailedReason(d.SecondaryError),
								Err: st.Err()})
					}
				}
			}
		}

		// 否则下载错误，没有下载的权限
		return sfs.WrapPrimaryError(sfs.DownloadFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.NoDownloadPermission,
				Err: fmt.Errorf("get temporary download url failed, err: %s", err.Error())})
	}
	exec.downloadUri = resp.Url
	exec.waitTimeMil = resp.WaitTimeMil
	if exec.fileSize <= exec.dl.balanceDownloadByteSize {
		// the file size is not big enough, download directly
		if e := exec.downloadDirectlyWithRetry(); e != nil {
			return sfs.WrapPrimaryError(sfs.DownloadFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.RetryDownloadFailed,
					Err: fmt.Errorf("download directly failed, err: %s", e.Error())})
		}

		return nil
	}

	size, yes, err := exec.isProviderSupportRangeDownload()
	if err != nil {
		logger.Warn("check if provider support range download failed", slog.Any("err", err.Error()))
	}

	if yes {
		if size != exec.fileSize {
			return sfs.WrapPrimaryError(sfs.DownloadFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.ValidateDownloadFailed,
					Err: fmt.Errorf("the to be download file size: %d is not as what we expected %d", size, exec.fileSize)})
		}

		if err := exec.downloadWithRange(); err != nil {
			return sfs.WrapPrimaryError(sfs.DownloadFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.DownloadChunkFailed,
					Err: fmt.Errorf("download with range failed, err: %s", err.Error())})
		}

		return nil
	}

	logger.Warn("provider does not support download with range policy, download directly now.")

	if err := exec.downloadDirectlyWithRetry(); err != nil {
		return sfs.WrapPrimaryError(sfs.DownloadFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.RetryDownloadFailed,
				Err: fmt.Errorf("download directly failed, err: %s", err.Error())})
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

// downloadDirectlyWithRetry download file directly with retry
func (exec *execDownload) downloadDirectlyWithRetry() error {
	logger.Debug("start download file directly",
		slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)),
		slog.Int64("waitTimeMil", exec.waitTimeMil))
	// wait before downloading, used for traffic control, avoid file storage service overload
	if exec.waitTimeMil > 0 {
		time.Sleep(time.Millisecond * time.Duration(exec.waitTimeMil))
	}

	// do download with retry
	retry := tools.NewRetryPolicy(1, [2]uint{500, 10000})
	maxRetryCount := 5
	for {
		if retry.RetryCount() >= uint32(maxRetryCount) {
			return fmt.Errorf("exec do download failed, retry count: %d", maxRetryCount)
		}
		if err := exec.downloadDirectly(requestAwaitResponseTimeoutSeconds); err != nil {
			logger.Error("exec do download failed", logger.ErrAttr(err), slog.Any("retry_count", retry.RetryCount()))
			retry.Sleep()
			continue
		}
		break
	}
	return nil
}

// downloadDirectly download file without range.
func (exec *execDownload) downloadDirectly(timeoutSeconds int) error {
	if err := exec.dl.sem.Acquire(exec.ctx, 1); err != nil {
		return fmt.Errorf("acquire semaphore failed, err: %s", err.Error())
	}

	logger.Info("start download file directly",
		slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)))

	defer exec.dl.sem.Release(1)

	start := time.Now()
	header := exec.header
	body, err := exec.doRequest(http.MethodGet, header, timeoutSeconds)
	if err != nil {
		return err
	}
	defer body.Close()

	if err := exec.write(body, exec.fileSize, 0); err != nil {
		return err
	}

	logger.Debug("download directly success",
		slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)),
		slog.Duration("cost", time.Since(start)),
	)

	return nil
}

func (exec *execDownload) downloadWithRange() error {
	logger.Info("start download file with range",
		slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)),
		slog.Int64("waitTimeMil", exec.waitTimeMil))
	// wait before downloading, used for traffic control, avoid file storage service overload
	if exec.waitTimeMil > 0 {
		time.Sleep(time.Millisecond * time.Duration(exec.waitTimeMil))
	}

	var start, end uint64
	batchSize := 2 * exec.dl.balanceDownloadByteSize
	// calculate the total parts to be downloaded
	totalParts := int(exec.fileSize / batchSize)
	if (exec.fileSize % batchSize) > 0 {
		totalParts++
	}

	var hitError error
	wg := sync.WaitGroup{}

	for part := 0; part < totalParts; part++ {
		start = uint64(part) * batchSize

		if part == totalParts-1 {
			end = exec.fileSize
		} else {
			end = start + batchSize
		}

		end--

		wg.Add(1)

		go func(pos int, from uint64, to uint64) {
			defer wg.Done()

			start := time.Now()
			if err := exec.downloadOneRangedPartWithRetry(from, to); err != nil {
				hitError = err
				logger.Error("download file part failed",
					slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)),
					slog.Int("part", pos),
					slog.Uint64("start", from),
					logger.ErrAttr(err))
				return
			}

			logger.Debug("download file range part success",
				slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)),
				slog.Int("part", pos),
				slog.Uint64("from", from),
				slog.Uint64("to", to),
				slog.Duration("cost", time.Since(start)))

		}(part, start, end)

	}

	wg.Wait()

	if hitError != nil {
		return hitError
	}

	logger.Debug("download full file success",
		slog.String("file", filepath.Join(exec.fileMeta.ConfigItemSpec.Path, exec.fileMeta.ConfigItemSpec.Name)))

	return nil
}

func (exec *execDownload) downloadOneRangedPartWithRetry(start uint64, end uint64) error {
	retry := tools.NewRetryPolicy(1, [2]uint{500, 10000})
	maxRetryCount := 5
	for {
		if retry.RetryCount() >= uint32(maxRetryCount) {
			return fmt.Errorf("download file part failed, retry count: %d", maxRetryCount)
		}
		if err := exec.downloadOneRangedPart(start, end); err != nil {
			logger.Error("download file part failed", logger.ErrAttr(err), slog.Any("retry_count", retry.RetryCount()))
			retry.Sleep()
			continue
		}
		break
	}
	return nil
}

func (exec *execDownload) downloadOneRangedPart(start uint64, end uint64) error {
	if start > end {
		return errors.New("invalid start or end to do range download")
	}

	if err := exec.dl.sem.Acquire(exec.ctx, 1); err != nil {
		return fmt.Errorf("acquire semaphore failed, err: %s", err.Error())
	}
	defer exec.dl.sem.Release(1)

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
	swap := swapPool.Get().(*[]byte)
	defer swapPool.Put(swap)
	for {
		select {
		case <-exec.ctx.Done():
			return fmt.Errorf("download context done, %s", exec.ctx.Err())

		default:
		}

		picked, err := body.Read(*swap)
		// we should always process the n > 0 bytes returned before
		// considering the error err. Doing so correctly handles I/O errors
		// that happen after reading some bytes and also both of the
		// allowed EOF behaviors.
		if picked > 0 {
			var cnt int
			switch exec.to {
			case DownloadToBytes:
				for i := 0; i < picked; i++ {
					exec.bytes[totalSize+uint64(i)] = (*swap)[i]
				}
			case DownloadToFile:
				cnt, err = exec.file.WriteAt((*swap)[0:picked], int64(start+totalSize))
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
	if len(tlsBytes.CertFileBytes) == 0 && len(tlsBytes.KeyFileBytes) == 0 {
		return &tls.Config{
			InsecureSkipVerify: tlsBytes.InsecureSkipVerify, // nolint
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
		InsecureSkipVerify: tlsBytes.InsecureSkipVerify, // nolint
		ClientCAs:          caPool,
		Certificates:       []tls.Certificate{tlsCert},
		ClientAuth:         tls.RequireAndVerifyClientCert,
	}, nil
}
