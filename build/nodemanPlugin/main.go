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

// program nodemanPlugin defines the bscp plugin main entry.
package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/config"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
	"github.com/TencentBlueKing/bscp-go/pkg/metrics"
)

const (
	defaultConfigPath    = "../etc/bkbscp.conf"
	pidFile              = "bkbscp.pid"
	unitSocketFile       = "bkbscp.sock"
	logFile              = "bkbscp.log"
	defaultLogMaxSize    = 500 // megabytes
	defaultLogMaxBackups = 3
	defaultLogMaxAge     = 15 // days
)

// ClientConfig 新增插件自定义配置
type ClientConfig struct {
	config.ClientConfig `json:",inline" mapstructure:",squash"`
	PidPath             string `json:"path.pid" mapstructure:"path.pid"`
	LogPath             string `json:"path.logs" mapstructure:"path.logs"`
	DataPath            string `json:"path.data" mapstructure:"path.data"`
}

// Validate validate the client config
func (c *ClientConfig) Validate() error {
	if err := c.ClientConfig.Validate(); err != nil {
		return err
	}

	if c.PidPath == "" {
		return fmt.Errorf("path.pid not set")
	}

	if c.LogPath == "" {
		return fmt.Errorf("path.logs not set")
	}

	return nil
}

var (
	configPath string
	conf       = new(ClientConfig)
	// gse插件使用.符号分割, viper特殊设置#以区分
	watchViper = viper.NewWithOptions(viper.KeyDelimiter("$"))
	rootCmd    = &cobra.Command{
		Use:   "bkbscp",
		Short: "bkbscp is a bscp nodeman plugin",
		Long:  `bkbscp is a bscp nodeman plugin`,
		RunE:  Watch,
	}
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

// initConf init the bscp client config
func initConf(v *viper.Viper) error {
	v.SetConfigFile(configPath)

	// 固定 yaml 格式
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file failed, err: %s", err.Error())
	}

	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config file failed, err: %s", err.Error())
	}

	logger.Debug("init conf", slog.String("conf", conf.String()))

	if err := conf.Validate(); err != nil {
		logger.Error("validate config failed", logger.ErrAttr(err))
		return err
	}
	return nil
}

// Watch run as a daemon to watch the config changes.
func Watch(cmd *cobra.Command, args []string) error {
	r := &lumberjack.Logger{
		Filename:   filepath.Join(conf.LogPath, logFile),
		MaxSize:    defaultLogMaxSize,
		MaxBackups: defaultLogMaxBackups,
		MaxAge:     defaultLogMaxAge,
	}
	defer r.Close() // nolint

	// 同时打印标准输出和日志文件
	w := io.MultiWriter(os.Stdout, r)
	setLogger(w)

	// print bscp banner
	fmt.Println(strings.TrimSpace(version.GetStartInfo()))

	logger.Info("use config file", "path", watchViper.ConfigFileUsed())

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(conf.Labels),
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
		return err
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
		if e := bscp.AddWatcher(
			handler.watchCallback, handler.App, handler.getSubscribeOptions()...); e != nil {
			logger.Error("add watch", logger.ErrAttr(e))
			return e
		}
	}
	if e := bscp.StartWatch(); e != nil {
		logger.Error("start watch", logger.ErrAttr(e))
		return err
	}

	return serveHttp()
}

// serveHttp nodeman插件绑定到本地的 sock/pid 文件
func serveHttp() error {
	// register metrics
	metrics.RegisterMetrics()
	http.Handle("/metrics", promhttp.Handler())

	if err := os.MkdirAll(conf.LogPath, os.ModeDir); err != nil {
		logger.Error("create log dir failed", logger.ErrAttr(err))
		return err
	}

	// 强制清理老的sock文件
	unitSocketPath := filepath.Join(conf.PidPath, unitSocketFile)
	_ = os.Remove(unitSocketPath)
	listen, err := net.Listen("unix", unitSocketPath)
	if err != nil {
		logger.Error("start http server failed", logger.ErrAttr(err))
		return err
	}

	if e := os.MkdirAll(conf.PidPath, os.ModeDir); e != nil {
		logger.Error("create pid dir failed", logger.ErrAttr(e))
		return e
	}

	pidPath := filepath.Join(conf.PidPath, pidFile)
	pid := os.Getpid()
	if e := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0664); e != nil {
		logger.Error("write to pid failed", logger.ErrAttr(e))
		return err
	}

	if e := http.Serve(listen, nil); e != nil {
		logger.Error("start http server failed", logger.ErrAttr(e))
		return err
	}

	return nil
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
	cobra.OnInitialize(func() {
		cobra.CheckErr(initConf(watchViper))
	})

	// 不开启 自动排序
	cobra.EnableCommandSorting = false
	// 不开启 completion 子命令
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// 添加版本
	rootCmd.Version = version.FormatVersion("", version.Row)
	rootCmd.SetVersionTemplate(`{{println .Version}}`)

	rootCmd.PersistentFlags().StringVarP(
		&configPath, "config", "c", defaultConfigPath, "config file path")
}

// setLogger 自定义日志
func setLogger(w io.Writer) {
	textHandler := slog.NewTextHandler(w, &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelInfo,
		ReplaceAttr: logger.ReplaceSourceAttr,
	})

	logger.SetHandler(textHandler)
}
