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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fsnotify/fsnotify"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/internal/config"
	"github.com/TencentBlueKing/bscp-go/internal/constant"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	conf = new(config.ClientConfig)
)

var (
	rootViper    = viper.New()
	pullViper    = viper.New()
	watchViper   = viper.New()
	getViper     = viper.New()
	getAppViper  = viper.New()
	getFileViper = viper.New()
	getKvViper   = viper.New()

	allVipers = []*viper.Viper{rootViper, pullViper, watchViper, getViper, getAppViper, getFileViper, getKvViper}
	getVipers = []*viper.Viper{getViper, getAppViper, getFileViper, getKvViper}
)

var (
	// !important: promise of compatibility
	// priority is same with viper: https://github.com/spf13/viper/blob/v1.18.2/viper.go#L146
	// priority: Command Flags > Environment Variables > Config File > Defaults

	// rootEnvs variable definition
	rootEnvs = map[string]string{
		"config_file": "BSCP_CONFIG",
		"log_level":   "log_level",
	}

	// commonEnvs variable definition, viper key => envName
	commonEnvs = map[string]string{
		"biz":                 "biz",
		"app":                 "app",
		"labels_str":          "labels",
		"labels_file":         "labels_file",
		"feed_addrs":          "feed_addrs",
		"token":               "token",
		"temp_dir":            "temp_dir",
		"enable_p2p_download": "enable_p2p_download",
		"bk_agent_id":         "bk_agent_id",
		"cluster_id":          "cluster_id",
		"pod_id":              "pod_id",
		"container_name":      "container_name",
	}

	watchEnvs = map[string]string{
		"port": "port",
	}
)

// ReloadMessage reload message with event and error
type ReloadMessage struct {
	Event  fsnotify.Event
	Labels map[string]string
	Error  error
}

// mustBindPFlag binds viper's a specific key to a pflag (as used by cobra)
func mustBindPFlag(v *viper.Viper, key string, flag *pflag.Flag) {
	if err := v.BindPFlag(key, flag); err != nil {
		panic(err)
	}
}

// initConf init the bscp client config
func initConf(v *viper.Viper) error {
	if v.GetString("config_file") != "" {
		if err := initFromConfFile(v); err != nil {
			return err
		}
	}

	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config file failed, err: %s", err.Error())
	}

	if err := conf.Update(); err != nil {
		return err
	}

	// debug日志打印配置信息，已屏蔽token敏感信息，便于调试和问题排查
	logger.Debug("init conf", slog.String("conf", conf.String()))
	return nil
}

func initFromConfFile(v *viper.Viper) error {
	c := v.GetString("config_file")
	// if config file path is same with default path and come from cmdline flag's default value,
	// which means the config file path is not set by cmdline flag, env etc. eg: not from `-c ./bscp.yaml`
	// then, if it does not exist, just ignore it
	if c == constant.DefaultConfFile && !v.IsSet("config_file") {
		if _, err := os.Stat(c); os.IsNotExist(err) {
			return nil
		}
	}

	v.SetConfigFile(c)
	// 固定 yaml 格式
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file failed, err: %s", err.Error())
	}
	return nil
}

func watchLabelsFile(ctx context.Context, path string, oldLabels map[string]string) (chan ReloadMessage, error) {
	watchChan := make(chan ReloadMessage)
	v := viper.New()
	v.SetConfigFile(path)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("new watcher failed, err: %s", err.Error())
	}
	if err := watcher.Add(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("add watcher for %s failed, err: %s", path, err.Error())
	}
	go func() {
		oldLabels := oldLabels
		for {
			select {
			case <-ctx.Done():
				logger.Info("watch labels file stopped because of ctx done", slog.String("file", path),
					logger.ErrAttr(ctx.Err()))
				if err := watcher.Close(); err != nil {
					logger.Warn("close watcher failed", logger.ErrAttr(err))
				}
				return
			case event := <-watcher.Events:
				msg := ReloadMessage{Event: event}
				if event.Op != fsnotify.Write {
					continue
				}

				absPath, err := filepath.Abs(event.Name)
				if err != nil {
					logger.Warn("get labels file absPath failed", logger.ErrAttr(err))
					continue
				}
				if absPath != path {
					continue
				}

				if err := v.ReadInConfig(); err != nil {
					msg.Error = fmt.Errorf("read labels file failed, err: %s", err.Error())
					watchChan <- msg
					continue
				}

				newLabels := make(map[string]string)
				if err := v.Unmarshal(&newLabels); err != nil {
					msg.Error = fmt.Errorf("unmarshal labels file failed, err: %s", err.Error())
					watchChan <- msg
					continue
				}

				if reflect.DeepEqual(newLabels, oldLabels) {
					continue
				}

				logger.Info("labels file changed, try reset labels",
					slog.String("file", path), slog.Any("old", oldLabels), slog.Any("new", newLabels))
				msg.Labels = newLabels
				watchChan <- msg
				oldLabels = newLabels
			case err := <-watcher.Errors:
				logger.Error("watcher error", logger.ErrAttr(err))
			}
		}
	}()
	return watchChan, nil
}

func readLabelsFile(path string) (map[string]string, error) {
	v := viper.New()
	v.SetConfigFile(path)
	fileLabels := make(map[string]string)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			logger.Warn("labels file not exist, skip read", slog.String("path", path))
			return fileLabels, nil
		}
		return nil, fmt.Errorf("stat labels file %s failed, err: %s", path, err.Error())
	}
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read labels file %s failed, err: %s", path, err.Error())
	}
	if err := v.Unmarshal(&fileLabels); err != nil {
		return nil, fmt.Errorf("unmarshal labels file %s failed, err: %s", path, err.Error())
	}
	return fileLabels, nil
}

// newTable 统一风格表格, 风格参考 kubectl
func newTable() *tablewriter.Table {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("   ") // pad with 3 space
	table.SetNoWhiteSpace(true)
	return table
}

// jsonOutput json风格输出
func jsonOutput(obj any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	return enc.Encode(obj)
}

// refineOutputTime 优化返回的时间显示, 时间格式固定 RFC3339 规范
func refineOutputTime(timeStr string) string {
	t, err := time.Parse(time.RFC3339, timeStr)

	var durStr string
	if err != nil {
		durStr = "N/A"
	} else {
		durStr = humanize.Time(t)
	}

	return durStr
}
