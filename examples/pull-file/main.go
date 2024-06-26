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

// pull file example for bscp sdk
package main

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/constant"
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
	if err = pullAppFiles(bscp, appName, opts); err != nil {
		slog.Error("pull", logger.ErrAttr(err))
		os.Exit(1)
	}
}

// pullAppFiles 拉取服务文件
func pullAppFiles(bscp client.Client, app string, opts []client.AppOption) error {
	release, err := bscp.PullFiles(app, opts...)
	if err != nil {
		return err
	}

	// 文件列表, 可以自定义操作，如查看content, 写入文件等
	for _, f := range release.FileItems {
		logger.Info("get event done", slog.Any("releaseID", release.ReleaseID), slog.Any("item", f))
	}

	return nil
}
