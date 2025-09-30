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

// watch file example for bscp sdk
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

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
		logger.Error("parse BSCP_BIZ", logger.ErrAttr(err))
		os.Exit(1)
	}

	labelsStr := os.Getenv("BSCP_LABELS")
	labels := map[string]string{}
	if labelsStr != "" {
		json.Unmarshal([]byte(labelsStr), &labels) // nolint
	}

	clientOpts := []client.Option{
		client.WithFeedAddrs(strings.Split(os.Getenv("BSCP_FEED_ADDRS"), ",")),
		client.WithBizID(uint32(biz)),
		client.WithToken(os.Getenv("BSCP_TOKEN")),
		client.WithLabels(labels),
	}
	if os.Getenv("BSCP_ENABLE_KV_CACHE") != "" {
		clientOpts = append(clientOpts, client.WithKvCache(client.KvCache{
			Enabled:     true,
			ThresholdMB: constant.DefaultKvCacheThresholdMB,
		}))
	}

	bscp, err := client.New(clientOpts...)
	if err != nil {
		logger.Error("init bscp client", logger.ErrAttr(err))
		os.Exit(1)
	}
	defer bscp.Close()

	appName := os.Getenv("BSCP_APP")
	opts := []client.AppOption{}
	if err = watchAppKV(bscp, appName, opts); err != nil {
		logger.Error("watch kv", logger.ErrAttr(err))
		os.Exit(1)
	}
}

type watcher struct {
	bscp client.Client
	app  string
}

// callback watch 回调函数
func (w *watcher) callback(release *client.Release) error {

	// kv 列表, 可以读取值
	for _, item := range release.KvItems {
		value, err := w.bscp.Get(w.app, item.Key)
		if err != nil {
			logger.Error("get value failed", slog.Any("releaseID", release.ReleaseID), slog.String("key", item.Key),
				logger.ErrAttr(err))
			continue
		}
		logger.Info("get value success", slog.Any("releaseID", release.ReleaseID), slog.String("key", item.Key),
			slog.String("value", value))
	}

	return nil
}

// watchAppKV watch 服务版本
func watchAppKV(bscp client.Client, app string, opts []client.AppOption) error {
	w := watcher{
		bscp: bscp,
		app:  app,
	}
	err := bscp.AddWatcher(w.callback, app, opts...)
	if err != nil {
		return err
	}

	if err := bscp.StartWatch(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	bscp.StopWatch()

	return nil
}
