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

// pull kv example for bscp sdk
package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

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
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}

	appName := os.Getenv("BSCP_APP")
	opts := []client.AppOption{}
	key := "key1"
	if err = pullAppKvs(bscp, appName, key, opts); err != nil {
		logger.Error("pull", logger.ErrAttr(err))
		os.Exit(1)
	}
	// 验证从缓存中获取value：此时可停掉feed-server服务端，使其不可用
	time.Sleep(time.Second * 10)
	if err = pullAppKvs(bscp, appName, key, opts); err != nil {
		logger.Error("pull", logger.ErrAttr(err))
		os.Exit(1)
	}
	// wait to see more kv cache statistics log
	time.Sleep(time.Second * 6)
}

// pullAppKvs 拉取 key 的值
func pullAppKvs(bscp client.Client, app string, key string, opts []client.AppOption) error {
	value, err := bscp.Get(app, key, opts...)
	if err != nil {
		return err
	}

	logger.Info("get value done", slog.String("key", key), slog.String("value", value))
	return nil
}
