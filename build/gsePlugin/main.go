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

// program nodeman defines the bscp node manager plugin main entry.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/config"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
	"github.com/TencentBlueKing/bscp-go/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// pluginName represents the name of the plugin
	pluginName = "bkbscp"
	// configPath represents the path to the configuration files
	configPath = "../etc/"
	// pidPath represents the path to the process identification (PID) file
	pidPath = "/var/run/gse"
)

var (
	logLevel   string
	conf       = new(config.ClientConfig)
	watchViper = viper.New()
	rootCmd    = &cobra.Command{
		Use:   "bkbscp",
		Short: "bkbscp is a bscp command line tool for gse plugin",
		Long:  `bkbscp is a bscp command line tool for gse plugin`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 设置日志等级
			level := logger.GetLevelByName(logLevel)
			logger.SetLevel(level)

			return nil
		},
		Run: Watch,
	}
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
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

	logger.Debug("init conf", slog.String("conf", conf.String()))
	return nil
}

func initFromConfFile(v *viper.Viper) error {
	c := v.GetString("config_file")
	v.SetConfigFile(c)
	// 固定 yaml 格式
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file failed, err: %s", err.Error())
	}
	return nil
}

// Watch run as a daemon to watch the config changes.
func Watch(cmd *cobra.Command, args []string) {
	// print bscp banner
	fmt.Println(strings.TrimSpace(version.GetStartInfo()))

	if err := initConf(watchViper); err != nil {
		logger.Error("init conf failed", logger.ErrAttr(err))
		os.Exit(1)
	}
	if conf.ConfigFile != "" {
		fmt.Println("use config file:", conf.ConfigFile)
	}
	if err := conf.Validate(); err != nil {
		logger.Error("validate config failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	confLabels := conf.Labels

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(confLabels),
		client.WithUID(conf.UID),
		client.WithFileCache(client.FileCache{
			Enabled:     conf.FileCache.Enabled,
			CacheDir:    conf.FileCache.CacheDir,
			ThresholdGB: conf.FileCache.ThresholdGB,
		}),
		client.WithEnableMonitorResourceUsage(conf.EnableMonitorResourceUsage),
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}

	for _, subscriber := range conf.Apps {
		handler := &WatchHandler{
			Biz:        conf.Biz,
			App:        subscriber.Name,
			Labels:     subscriber.Labels,
			UID:        subscriber.UID,
			Lock:       sync.Mutex{},
			TempDir:    conf.TempDir,
			AppTempDir: path.Join(conf.TempDir, strconv.Itoa(int(conf.Biz)), subscriber.Name),
			bscp:       bscp,
		}
		if err := bscp.AddWatcher(handler.watchCallback, handler.App, handler.getSubscribeOptions()...); err != nil {
			logger.Error("add watch", logger.ErrAttr(err))
			os.Exit(1)
		}
	}
	if e := bscp.StartWatch(); e != nil {
		logger.Error("start watch", logger.ErrAttr(e))
		os.Exit(1)
	}

	serveHttp()
}

// serveHttp gse插件绑定到本地的 sock 文件
func serveHttp() {
	// register metrics
	metrics.RegisterMetrics()
	http.Handle("/metrics", promhttp.Handler())
	if e := http.ListenAndServe(fmt.Sprintf(":%d", conf.Port), nil); e != nil {
		logger.Error("start http server failed", logger.ErrAttr(e))
		os.Exit(1)
	}
}

// WatchHandler watch handler
type WatchHandler struct {
	// Biz BSCP biz id
	Biz uint32
	// App BSCP app name
	App string
	// Labels instance labels
	Labels map[string]string
	// UID instance unique uid
	UID string
	// TempDir bscp temporary directory
	TempDir string
	// AppTempDir app temporary directory
	AppTempDir string
	// Lock lock for concurrent callback
	Lock sync.Mutex
	bscp client.Client
}

func (w *WatchHandler) watchCallback(release *client.Release) error { // nolint
	w.Lock.Lock()
	defer func() {
		w.Lock.Unlock()
	}()

	release.AppDir = w.AppTempDir
	release.TempDir = w.TempDir
	release.BizID = w.Biz
	release.ClientMode = sfs.Watch

	if err := release.Execute(release.ExecuteHook(&client.PreScriptStrategy{}), release.UpdateFiles(),
		release.ExecuteHook(&client.PostScriptStrategy{}), release.UpdateMetadata()); err != nil {
		return err
	}
	// TODO: 6.2 call the callback notify api
	logger.Info("watch release change success", slog.Any("currentReleaseID", release.ReleaseID))
	return nil
}

func (w *WatchHandler) getSubscribeOptions() []client.AppOption {
	var options []client.AppOption
	options = append(options, client.WithAppLabels(w.Labels))
	options = append(options, client.WithAppUID(w.UID))
	return options
}

func init() {
	// 不开启 自动排序
	cobra.EnableCommandSorting = false
	// 不开启 completion 子命令
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.PersistentFlags().StringVarP(
		&logLevel, "log-level", "", "", "log filtering level, One of: debug|info|warn|error. (default info)")
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")

}
