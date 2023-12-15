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

// vk ctl example for bscp sdk
package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
	"github.com/TencentBlueKing/bscp-go/types"
)

var rootCmd = &cobra.Command{
	Use:   "kv-ctl",
	Short: "bscp kv ctl",
	Run: func(cmd *cobra.Command, args []string) {
		execute()
	},
}

var (
	watchMode bool
	keys      string
	logLevel  string
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&watchMode, "watch", "w", false, "use watch mode")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log.level", "", "warn", "log filtering level.")
	rootCmd.PersistentFlags().StringVarP(&keys, "keys", "k", "", "use commas to separate, like key1,key2. (watch mode empty key will get all values)")
}

func main() {
	rootCmd.Execute()
}

func execute() {
	level := slog.LevelInfo
	switch logLevel {
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	default:
		level = slog.LevelWarn
	}

	// 设置日志自定义 Handler
	logger.SetHandler(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))

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

	bscp, err := client.New(
		client.WithFeedAddr(os.Getenv("BSCP_FEED_ADDR")),
		client.WithBizID(uint32(biz)),
		client.WithToken(os.Getenv("BSCP_TOKEN")),
		client.WithLabels(labels),
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}

	appName := os.Getenv("BSCP_APP")
	opts := []types.AppOption{}
	keySlice := strings.Split(keys, ",")
	if watchMode {
		if err = watchAppKV(bscp, appName, keySlice, opts); err != nil {
			logger.Error("watch", logger.ErrAttr(err))
			os.Exit(1)
		}
	} else {
		result := map[string]string{}

		for _, key := range keySlice {
			value, err := bscp.Get(appName, key, opts...)
			if err != nil {
				continue
			}
			result[key] = value
		}

		json.NewEncoder(os.Stdout).Encode(result) // nolint
	}
}

type watcher struct {
	bscp   client.Client
	app    string
	keyMap map[string]struct{}
}

// callback watch 回调函数
func (w *watcher) callback(release *types.Release) error {
	result := map[string]string{}

	// kv 列表, 可以读取值
	for _, item := range release.KvItems {
		value, err := w.bscp.Get(w.app, item.Key)
		if err != nil {
			logger.Error("get value failed: %d, %v, err: %s", release.ReleaseID, item.Key, err)
			continue
		}
		logger.Info("get value success: %d, %v, %s", release.ReleaseID, item.Key, value)

		// key匹配或者为空时，输出
		if _, ok := w.keyMap[item.Key]; ok || len(keys) == 0 {
			result[item.Key] = value
		}
	}

	json.NewEncoder(os.Stdout).Encode(result) // nolint

	return nil
}

// watchAppKV watch 服务版本
func watchAppKV(bscp client.Client, app string, keys []string, opts []types.AppOption) error {
	keyMap := map[string]struct{}{}
	for _, v := range keys {
		keyMap[v] = struct{}{}
	}

	w := watcher{
		bscp:   bscp,
		app:    app,
		keyMap: keyMap,
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
