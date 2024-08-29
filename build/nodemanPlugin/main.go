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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shirou/gopsutil/v3/process"
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

var (
	configPath string
	conf       = new(pluginConfig)
	// gse插件使用.符号分割, viper特殊设置$以区分
	watchViper = viper.NewWithOptions(viper.KeyDelimiter("$"))
	rootCmd    = &cobra.Command{
		Use:   "bkbscp",
		Short: "bkbscp is a bscp nodeman plugin",
		Long:  `bkbscp is a bscp nodeman plugin`,
		RunE:  Watch,
	}
)

// pluginConfig 新增插件自定义配置
type pluginConfig struct {
	config.ClientConfig `json:",inline" mapstructure:",squash"`
	PidPath             string `json:"path.pid" mapstructure:"path.pid"`
	LogPath             string `json:"path.logs" mapstructure:"path.logs"`
	DataPath            string `json:"path.data" mapstructure:"path.data"`
	HostIP              string `json:"hostip" mapstructure:"hostip"`             // cmdb 内网 IP
	CloudId             int    `json:"cloudid" mapstructure:"cloudid"`           // cmdb 云区域ID
	HostId              int    `json:"hostid" mapstructure:"hostid"`             // cmdb hostid
	HostIdPath          string `json:"host_id_path" mapstructure:"host_id_path"` // cmdb 主机元数据文件路径
	BKAgentId           string `json:"bk_agent_id" mapstructure:"bk_agent_id"`   // gse agent id, 可能为空
}

// Validate validate the client config
func (c *pluginConfig) Validate() error {
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

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

// initConf init the bscp client config
func initConf(v *viper.Viper) error {
	v.SetConfigFile(configPath)

	// 固定 yaml 格式
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config file: %w", err)
	}

	if err := conf.Update(); err != nil {
		return err
	}

	if err := conf.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
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

	if err := ensurePid(); err != nil {
		logger.Error("ensure pid failed", logger.ErrAttr(err))
		return err
	}

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(conf.Labels),
		client.WithUID(conf.UID),
		client.WithP2PDownload(conf.EnableP2PDownload),
		client.WithBkAgentID(conf.BkAgentID),
		client.WithFileCache(client.FileCache{
			Enabled:     conf.FileCache.Enabled,
			CacheDir:    conf.FileCache.CacheDir,
			ThresholdGB: conf.FileCache.ThresholdGB,
		}),
		client.WithEnableMonitorResourceUsage(conf.EnableMonitorResourceUsage),
		client.WithTextLineBreak(conf.TextLineBreak),
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		return err
	}

	for _, subscriber := range conf.Apps {
		handler := &WatchHandler{
			Biz:           conf.Biz,
			App:           subscriber.Name,
			Labels:        subscriber.Labels,
			UID:           subscriber.UID,
			ConfigMatches: subscriber.ConfigMatches,
			Lock:          sync.Mutex{},
			TempDir:       conf.TempDir,
			AppTempDir:    filepath.Join(conf.TempDir, strconv.Itoa(int(conf.Biz)), subscriber.Name),
			bscp:          bscp,
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

	if e := http.Serve(listen, nil); e != nil {
		logger.Error("start http server failed", logger.ErrAttr(e))
		return err
	}

	return nil
}

func checkProcess(pidPath string) error {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return err
	}

	p, err := process.NewProcess(int32(pid))
	if err != nil {
		if errors.Is(err, process.ErrorProcessNotRunning) {
			return nil
		}
		return err
	}

	return fmt.Errorf("pid %d still alive, stop first", p.Pid)
}

// ensurePid 检查pid文件，如果里面的进程存活, 退出启动; 如果不存在，覆盖写入
func ensurePid() error {
	if err := os.MkdirAll(conf.PidPath, os.ModeDir); err != nil {
		return fmt.Errorf("create pid dir: %w", err)
	}

	pidPath := filepath.Join(conf.PidPath, pidFile)
	if err := checkProcess(pidPath); err != nil {
		return fmt.Errorf("check pid: %w", err)
	}

	pid := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("write to pid: %w", err)
	}

	logger.Info("write to pid success", "path", pidPath, "pid", pid)

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
	// ConfigMatches app config item's match conditions
	ConfigMatches []string
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
	defer w.Lock.Unlock()

	release.AppDir = w.AppTempDir
	release.TempDir = w.TempDir
	release.BizID = w.Biz
	release.ClientMode = sfs.Watch

	if err := release.Execute(release.ExecuteHook(&client.PreScriptStrategy{}), release.UpdateFiles(),
		release.ExecuteHook(&client.PostScriptStrategy{}), release.UpdateMetadata()); err != nil {
		return err
	}
	logger.Info("watch release change success", slog.Any("currentReleaseID", release.ReleaseID))
	return nil
}

func (w *WatchHandler) getSubscribeOptions() []client.AppOption {
	var options []client.AppOption
	options = append(options, client.WithAppLabels(w.Labels))
	options = append(options, client.WithAppUID(w.UID))
	options = append(options, client.WithAppConfigMatch(w.ConfigMatches))
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
