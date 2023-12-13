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

	"log/slog"

	"github.com/TencentBlueKing/bscp-go/cli/config"
	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/logger"
	"github.com/TencentBlueKing/bscp-go/option"
	"github.com/TencentBlueKing/bscp-go/types"
)

func main() {
	// 设置日志自定义 Handler
	// logger.SetHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	// 初始化配置信息, 按需修改
	bizStr := os.Getenv("BSCP_BIZ")
	biz, err := strconv.ParseInt(bizStr, 10, 64)
	if err != nil {
		logger.Error("invalid BSCP_BIZ", logger.ErrAttr(err))
		os.Exit(1)
	}

	labelsStr := os.Getenv("BSCP_LABELS")
	labels := map[string]string{}
	if labelsStr != "" {
		json.Unmarshal([]byte(labelsStr), &labels) // nolint
	}

	conf := &config.ClientConfig{
		FeedAddrs: strings.Split(os.Getenv("BSCP_FEED_ADDRS"), ","),
		Biz:       uint32(biz),
		Token:     os.Getenv("BSCP_TOKEN"),
		Labels:    labels,
	}

	bscp, err := client.New(
		option.FeedAddrs(conf.FeedAddrs),
		option.BizID(conf.Biz),
		option.Token(conf.Token),
		option.Labels(conf.Labels),
	)
	if err != nil {
		logger.Error("init bscp client", logger.ErrAttr(err))
		os.Exit(1)
	}

	appName := os.Getenv("BSCP_APP")
	opts := []option.AppOption{}
	if err = watchAppKV(bscp, appName, opts); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

type watcher struct {
	bscp client.Client
	app  string
}

// callback watch 回调函数
func (w *watcher) callback(release *types.Release) error {

	// kv 列表, 可以读取值
	for _, item := range release.KvItems {
		value, err := w.bscp.Get(w.app, item.Key)
		if err != nil {
			logger.Error("get value failed", slog.Any("releaseID", release.ReleaseID), slog.String("key", item.Key), logger.ErrAttr(err))
			continue
		}
		logger.Info("get value success", slog.Any("releaseID", release.ReleaseID), slog.String("key", item.Key), slog.String("value", value))
	}

	return nil
}

// watchAppKV watch 服务版本
func watchAppKV(bscp client.Client, app string, opts []option.AppOption) error {
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
