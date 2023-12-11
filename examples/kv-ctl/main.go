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

	"bscp.io/pkg/logs"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bscp-go/cli/config"
	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/option"
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
	watchMode  bool
	keys       string
	logEnabled bool
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&watchMode, "watch", "w", false, "use watch mode")
	rootCmd.PersistentFlags().BoolVarP(&logEnabled, "log.enabled", "", false, "enable log")
	rootCmd.PersistentFlags().StringVarP(&keys, "keys", "k", "", "use commas to separate, like key1,key2. (watch mode empty key will get all values)")
}

func main() {
	rootCmd.Execute()
}

func execute() {
	if logEnabled {
		logs.InitLogger(logs.LogConfig{ToStdErr: true, LogLineMaxSize: 1000})
	}

	// 初始化配置信息, 按需修改
	bizStr := os.Getenv("BSCP_BIZ")
	biz, err := strconv.ParseInt(bizStr, 10, 64)
	if err != nil {
		logs.Errorf(err.Error())
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
		logs.Errorf(err.Error())
		os.Exit(1)
	}

	appName := os.Getenv("BSCP_APP")
	opts := []option.AppOption{}
	keySlice := strings.Split(keys, ",")
	if watchMode {
		if err = watchAppKV(bscp, appName, keySlice, opts); err != nil {
			logs.Errorf(err.Error())
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
			logs.Errorf("get value failed: %d, %v, err: %s", release.ReleaseID, item.Key, err)
			continue
		}
		logs.Infof("get value success: %d, %v, %s", release.ReleaseID, item.Key, value)

		// key匹配或者为空时，输出
		if _, ok := w.keyMap[item.Key]; ok || len(keys) == 0 {
			result[item.Key] = value
		}
	}

	json.NewEncoder(os.Stdout).Encode(result) // nolint

	return nil
}

// watchAppKV watch 服务版本
func watchAppKV(bscp client.Client, app string, keys []string, opts []option.AppOption) error {
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
