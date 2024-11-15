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

// download file bench test
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/constant"
	"github.com/TencentBlueKing/bscp-go/internal/downloader"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

func main() {
	// 设置日志等级为debug
	level := logger.GetLevelByName("debug")
	logger.SetLevel(level)

	// 设置日志自定义 Handler
	// logger.SetHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	// 在线服务, 可设置 metrics
	// metrics.RegisterMetrics()
	// http.Handle("/metrics", promhttp.Handler())

	// 初始化配置信息, 按需修改
	bizStr := os.Getenv("BSCP_BIZ")
	biz, err := strconv.ParseInt(bizStr, 10, 64)
	if err != nil {
		slog.Error("parse BSCP_BIZ", logger.ErrAttr(err))
		os.Exit(1)
	}

	// goroutine number for concurrency
	var gnum int64 = 10
	gnumStr := os.Getenv("GNUM")
	if gnumStr != "" {
		gnum, err = strconv.ParseInt(gnumStr, 10, 64)
		if err != nil {
			slog.Error("parse GNUM", logger.ErrAttr(err))
			os.Exit(1)
		}
	}

	clientOpts := []client.Option{
		client.WithFeedAddrs(strings.Split(os.Getenv("BSCP_FEED_ADDRS"), ",")),
		client.WithBizID(uint32(biz)),
		client.WithToken(os.Getenv("BSCP_TOKEN")),
	}
	if os.Getenv("BSCP_ENABLE_FILE_CACHE") != "" {
		clientOpts = append(clientOpts, client.WithFileCache(client.FileCache{
			Enabled:     true,
			CacheDir:    constant.DefaultFileCacheDir,
			ThresholdGB: constant.DefaultCacheThresholdGB,
		}))
	}

	bscp, err := client.New(clientOpts...)
	if err != nil {
		slog.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}

	appName := os.Getenv("BSCP_APP")
	opts := []client.AppOption{}
	release, err := bscp.PullFiles(appName, opts...)
	if err != nil {
		slog.Error("pull app files failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	slog.Info("start downloading file", slog.Int64("concurrency", gnum))
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < int(gnum); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err = downloadAppFiles(release); err != nil {
				slog.Error("download app files failed", logger.ErrAttr(err))
			}
		}()
	}
	wg.Wait()

	costTime := time.Since(start).Seconds()
	slog.Info("download app files finished", slog.Int64("success", success), slog.Int64("fail", fail),
		slog.Float64("cost_time_seconds", costTime))

	// 持续阻塞，便于观察对比客户端进程负载状况
	select {}
}

var success, fail int64

// downloadAppFiles 下载服务文件
func downloadAppFiles(release *client.Release) error {
	for _, c := range release.FileItems {
		bytes := make([]byte, c.FileMeta.ContentSpec.ByteSize)

		if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
			c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToBytes, bytes, ""); err != nil {
			atomic.AddInt64(&fail, 1)
			return err
		}
		atomic.AddInt64(&success, 1)
		logger.Debug("get file content by downloading from repo success",
			slog.String("file", filepath.Join(c.Path, c.Name)))
	}

	return nil
}
