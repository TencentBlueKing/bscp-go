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
	"fmt"
	"net/http"
	_ "net/http/pprof" // nolint
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sfs "github.com/TencentBlueKing/bk-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bscp/pkg/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/constant"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
	"github.com/TencentBlueKing/bscp-go/pkg/metrics"
)

var (
	// WatchCmd command to watch app files
	WatchCmd = &cobra.Command{
		Use:   "watch",
		Short: "watch release then pull file, exec hooks",
		Long:  `watch release then pull file, exec hooks`,
		Run:   Watch,
	}
)

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

	labels := conf.Labels
	r := &refinedLabelsFile{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	if conf.LabelsFile != "" {
		r, err = refineLabelsFile(ctx, conf.LabelsFile, labels)
		if err != nil {
			logger.Error("refine labels file", logger.ErrAttr(err))
			os.Exit(1) //nolint:gocritic
		}
		labels = r.mergeLabels
	}

	// 设置pod name
	if version.CLIENTTYPE == string(sfs.Sidecar) {
		conf.Labels["pod_name"] = os.Getenv("HOSTNAME")
	}

	bscp, err := newWatchClient(labels)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
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
		if err := bscp.AddWatcher(handler.watchCallback, handler.App, handler.getSubscribeOptions()...); err != nil {
			logger.Error("add watch", logger.ErrAttr(err))
			os.Exit(1)
		}
	}

	if conf.EnableP2PDownload {
		// enable gse p2p download, wait for container to report itself's containerID to bcs storage
		time.Sleep(5 * time.Second)
	}

	if e := bscp.StartWatch(); e != nil {
		logger.Error("start watch", logger.ErrAttr(e))
		os.Exit(1)
	}

	go func() {
		if r.reloadChan == nil {
			return
		}
		for {
			msg := <-r.reloadChan
			if msg.Error != nil {
				logger.Error("reset labels failed", logger.ErrAttr(msg.Error))
				continue
			}
			bscp.ResetLabels(util.MergeLabels(conf.Labels, msg.Labels))
			logger.Info("reset labels success, will reload watch")
		}
	}()

	serveHttp()
}

func newWatchClient(labels map[string]string) (client.Client, error) {
	return client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(labels),
		client.WithUID(conf.UID),
		client.WithP2PDownload(conf.EnableP2PDownload),
		client.WithBkAgentID(conf.BkAgentID),
		client.WithClusterID(conf.ClusterID),
		client.WithPodID(conf.PodID),
		client.WithContainerName(conf.ContainerName),
		client.WithFileCache(client.FileCache{
			Enabled:     conf.FileCache.Enabled,
			CacheDir:    conf.FileCache.CacheDir,
			ThresholdGB: conf.FileCache.ThresholdGB,
		}),
		client.WithKvCache(client.KvCache{
			Enabled:     conf.KvCache.Enabled,
			ThresholdMB: conf.KvCache.ThresholdMB,
		}),
		client.WithEnableMonitorResourceUsage(conf.EnableMonitorResourceUsage),
		client.WithTextLineBreak(conf.TextLineBreak),
	)
}

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

type refinedLabelsFile struct {
	absPath     string
	reloadChan  chan ReloadMessage
	mergeLabels map[string]string
}

func refineLabelsFile(ctx context.Context, path string, confLabels map[string]string) (*refinedLabelsFile, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("read labels file path failed, err: %s", err.Error())
	}

	labelsFromFile, err := readLabelsFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read labels file failed, err: %s", err.Error())
	}

	reloadChan, err := watchLabelsFile(ctx, absPath, labelsFromFile)
	if err != nil {
		return nil, fmt.Errorf("watch labels file failed, err: %s", err.Error())
	}
	logger.Info("watching labels file", slog.String("file", absPath))

	mergeLabels := util.MergeLabels(confLabels, labelsFromFile)
	r := &refinedLabelsFile{
		absPath:     absPath,
		reloadChan:  reloadChan,
		mergeLabels: mergeLabels,
	}
	return r, nil
}

func (w *WatchHandler) watchCallback(release *client.Release) error {
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
	// !important: promise of compatibility
	WatchCmd.Flags().SortFlags = false

	WatchCmd.Flags().StringP("feed-addrs", "f", "", "feed server address, eg: 'bscp-feed.example.com:9510'")
	mustBindPFlag(watchViper, "feed_addrs", WatchCmd.Flags().Lookup("feed-addrs"))
	WatchCmd.Flags().IntP("biz", "b", 0, "biz id")
	mustBindPFlag(watchViper, "biz", WatchCmd.Flags().Lookup("biz"))
	WatchCmd.Flags().StringP("app", "a", "", "app name")
	mustBindPFlag(watchViper, "app", WatchCmd.Flags().Lookup("app"))
	WatchCmd.Flags().StringP("token", "t", "", "sdk token")
	mustBindPFlag(watchViper, "token", WatchCmd.Flags().Lookup("token"))
	WatchCmd.Flags().StringP("labels", "l", "", "labels")
	mustBindPFlag(watchViper, "labels_str", WatchCmd.Flags().Lookup("labels"))
	WatchCmd.Flags().StringP("labels-file", "", "", "labels file path")
	mustBindPFlag(watchViper, "labels_file", WatchCmd.Flags().Lookup("labels-file"))
	WatchCmd.Flags().StringP("config-matches", "m", "", "app config item's match conditions，eg:'/etc/a*,/etc/b*'")
	mustBindPFlag(watchViper, "config_matches", WatchCmd.Flags().Lookup("config-matches"))
	// TODO: set client UID
	WatchCmd.Flags().StringP("temp-dir", "d", constant.DefaultTempDir, "bscp temp dir")
	mustBindPFlag(watchViper, "temp_dir", WatchCmd.Flags().Lookup("temp-dir"))
	WatchCmd.Flags().IntP("port", "p", constant.DefaultHttpPort, "sidecar http port")
	mustBindPFlag(watchViper, "port", WatchCmd.Flags().Lookup("port"))
	WatchCmd.Flags().BoolP("enable-p2p-download", "", false, "enable p2p download or not")
	mustBindPFlag(watchViper, "enable-p2p-download", WatchCmd.Flags().Lookup("enable-p2p-download"))
	WatchCmd.Flags().StringP("bk-agent-id", "", "", "gse agent id")
	mustBindPFlag(watchViper, "bk_agent_id", WatchCmd.Flags().Lookup("bk-agent-id"))
	WatchCmd.Flags().StringP("cluster-id", "", "", "cluster id")
	mustBindPFlag(watchViper, "cluster_id", WatchCmd.Flags().Lookup("cluster-id"))
	WatchCmd.Flags().StringP("pod-id", "", "", "pod id")
	mustBindPFlag(watchViper, "pod_id", WatchCmd.Flags().Lookup("pod-id"))
	WatchCmd.Flags().StringP("container-name", "", "", "container name")
	mustBindPFlag(watchViper, "container_name", WatchCmd.Flags().Lookup("container-name"))
	WatchCmd.Flags().BoolP("file-cache-enabled", "", constant.DefaultFileCacheEnabled, "enable file cache or not")
	mustBindPFlag(watchViper, "file_cache.enabled", WatchCmd.Flags().Lookup("file-cache-enabled"))
	WatchCmd.Flags().StringP("file-cache-dir", "", constant.DefaultFileCacheDir, "bscp file cache dir")
	mustBindPFlag(watchViper, "file_cache.cache_dir", WatchCmd.Flags().Lookup("file-cache-dir"))
	WatchCmd.Flags().Float64P("cache-threshold-gb", "", constant.DefaultCacheThresholdGB,
		"bscp file cache threshold gigabyte")
	mustBindPFlag(watchViper, "file_cache.threshold_gb", WatchCmd.Flags().Lookup("cache-threshold-gb"))
	WatchCmd.Flags().BoolP("kv-cache-enabled", "", constant.DefaultKvCacheEnabled, "enable kv cache or not")
	mustBindPFlag(watchViper, "kv_cache.enabled", WatchCmd.Flags().Lookup("kv-cache-enabled"))
	WatchCmd.Flags().Float64P("kv-cache-threshold-mb", "", constant.DefaultKvCacheThresholdMB,
		"bscp kv cache threshold megabyte in memory")
	mustBindPFlag(watchViper, "kv_cache.threshold_mb", WatchCmd.Flags().Lookup("kv-cache-threshold-mb"))
	WatchCmd.Flags().BoolP("enable-resource", "e", true, "enable report resource usage")
	mustBindPFlag(watchViper, "enable_resource", WatchCmd.Flags().Lookup("enable-resource"))
	WatchCmd.Flags().StringP("text-line-break", "", "", "text line break, default as LF")
	mustBindPFlag(watchViper, "text_line_break", WatchCmd.Flags().Lookup("text-line-break"))

	envs := map[string]string{}
	for key, envName := range commonEnvs {
		envs[key] = envName
	}
	for key, envName := range watchEnvs {
		envs[key] = envName
	}
	for key, envName := range envs {
		// bind env variable with viper
		if err := watchViper.BindEnv(key, envName); err != nil {
			panic(err)
		}
		// add env info for cmdline flags
		if f := WatchCmd.Flags().Lookup(strings.ReplaceAll(key, "_", "-")); f != nil {
			f.Usage = fmt.Sprintf("%v [env %v]", f.Usage, envName)
		}
	}
}
