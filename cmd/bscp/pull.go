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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	sfs "github.com/TencentBlueKing/bk-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bscp/pkg/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/constant"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/internal/util/process_collect"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	// PullCmd command to pull app files
	PullCmd = &cobra.Command{
		Use:   "pull",
		Short: "pull file to temp-dir and exec hooks",
		Long:  `pull file to temp-dir and exec hooks`,
		Run:   Pull,
	}
)

// Pull executes the pull command.
func Pull(cmd *cobra.Command, args []string) {
	// print bscp banner
	fmt.Println(strings.TrimSpace(version.GetStartInfo()))

	if err := initConf(pullViper); err != nil {
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

	// 设置pod name
	if version.CLIENTTYPE == string(sfs.Sidecar) {
		conf.Labels["pod_name"] = os.Getenv("HOSTNAME")
	}

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(conf.Labels),
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
		client.WithTextLineBreak(conf.TextLineBreak),
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 是否采集/监控资源使用率
	if conf.EnableMonitorResourceUsage {
		go process_collect.NewProcessCollector(ctx)
	}

	if conf.EnableP2PDownload {
		// enable gse p2p download, wait for container to report itself's containerID to bcs storage
		time.Sleep(5 * time.Second)
	}

	for _, app := range conf.Apps {
		opts := []client.AppOption{}
		opts = append(opts, client.WithAppConfigMatch(app.ConfigMatches))
		opts = append(opts, client.WithAppLabels(app.Labels))
		opts = append(opts, client.WithAppUID(app.UID))
		if err = pullAppFiles(ctx, bscp, conf.TempDir, conf.Biz, app.Name, opts); err != nil {
			cancel()
			logger.Error("pull files failed", logger.ErrAttr(err))
			os.Exit(1)
		}
	}
	cancel()
}

func pullAppFiles(ctx context.Context, bscp client.Client, tempDir string, biz uint32, app string, opts []client.AppOption) error { // nolint

	// 1. prepare app workspace dir
	appDir := filepath.Join(tempDir, strconv.Itoa(int(biz)), app)
	if e := os.MkdirAll(appDir, os.ModePerm); e != nil {
		return e
	}

	release, err := bscp.PullFiles(app, opts...)
	if err != nil {
		return err
	}

	release.AppDir = appDir
	release.TempDir = tempDir
	release.BizID = biz
	release.ClientMode = sfs.Pull
	// 生成事件ID
	release.CursorID = util.GenerateCursorID(biz)
	release.SemaphoreCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-release.SemaphoreCh:
				successDownloads := atomic.LoadInt32(&release.AppMate.DownloadFileNum)
				successFileSize := atomic.LoadUint64(&release.AppMate.DownloadFileSize)
				release.AppMate.DownloadFileNum = successDownloads
				release.AppMate.DownloadFileSize = successFileSize
			}
		}
	}()

	// 1.执行前置脚本
	// 2.更新文件
	// 3.执行后置脚本
	// 4.更新Metadata
	if err = release.Execute(release.ExecuteHook(&client.PreScriptStrategy{}), release.UpdateFiles(),
		release.ExecuteHook(&client.PostScriptStrategy{}), release.UpdateMetadata()); err != nil {
		return err
	}
	logger.Info("pull files success", slog.Any("releaseID", release.ReleaseID))
	return nil
}

func init() {
	// !important: promise of compatibility
	PullCmd.Flags().SortFlags = false
	PullCmd.Flags().StringP("feed-addrs", "f", "", "feed server address, eg: 'bscp-feed.example.com:9510'")
	mustBindPFlag(pullViper, "feed_addrs", PullCmd.Flags().Lookup("feed-addrs"))
	PullCmd.Flags().IntP("biz", "b", 0, "biz id")
	mustBindPFlag(pullViper, "biz", PullCmd.Flags().Lookup("biz"))
	PullCmd.Flags().StringP("app", "a", "", "app name")
	mustBindPFlag(pullViper, "app", PullCmd.Flags().Lookup("app"))
	PullCmd.Flags().StringP("token", "t", "", "sdk token")
	mustBindPFlag(pullViper, "token", PullCmd.Flags().Lookup("token"))
	PullCmd.Flags().StringP("labels", "l", "", "labels")
	mustBindPFlag(pullViper, "labels_str", PullCmd.Flags().Lookup("labels"))
	PullCmd.Flags().StringP("labels-file", "", "", "labels file path")
	mustBindPFlag(pullViper, "labels_file", PullCmd.Flags().Lookup("labels-file"))
	PullCmd.Flags().StringP("config-matches", "m", "", "app config item's match conditions，eg:'/etc/a*,/etc/b*'")
	mustBindPFlag(pullViper, "config_matches", PullCmd.Flags().Lookup("config-matches"))
	// TODO: set client UID
	PullCmd.Flags().StringP("temp-dir", "d", constant.DefaultTempDir, "bscp temp dir")
	mustBindPFlag(pullViper, "temp_dir", PullCmd.Flags().Lookup("temp-dir"))
	PullCmd.Flags().BoolP("enable-p2p-download", "", false, "enable p2p download or not")
	mustBindPFlag(pullViper, "enable_p2p_download", PullCmd.Flags().Lookup("enable-p2p-download"))
	PullCmd.Flags().StringP("bk-agent-id", "", "", "gse agent id")
	mustBindPFlag(pullViper, "bk_agent_id", PullCmd.Flags().Lookup("bk-agent-id"))
	PullCmd.Flags().StringP("cluster-id", "", "", "cluster id")
	mustBindPFlag(pullViper, "cluster_id", PullCmd.Flags().Lookup("cluster-id"))
	PullCmd.Flags().StringP("pod-id", "", "", "pod id")
	mustBindPFlag(pullViper, "pod_id", PullCmd.Flags().Lookup("pod-id"))
	PullCmd.Flags().StringP("container-name", "", "", "container name")
	mustBindPFlag(pullViper, "container_name", PullCmd.Flags().Lookup("container-name"))
	PullCmd.Flags().BoolP("file-cache-enabled", "", constant.DefaultFileCacheEnabled, "enable file cache or not")
	mustBindPFlag(pullViper, "file_cache.enabled", PullCmd.Flags().Lookup("file-cache-enabled"))
	PullCmd.Flags().StringP("file-cache-dir", "", constant.DefaultFileCacheDir, "bscp file cache dir")
	mustBindPFlag(pullViper, "file_cache.cache_dir", PullCmd.Flags().Lookup("file-cache-dir"))
	PullCmd.Flags().Float64P("cache-threshold-gb", "", constant.DefaultCacheThresholdGB,
		"bscp file cache threshold gigabyte")
	mustBindPFlag(pullViper, "file_cache.threshold_gb", PullCmd.Flags().Lookup("cache-threshold-gb"))
	PullCmd.Flags().BoolP("enable-resource", "e", true, "enable report resource usage")
	mustBindPFlag(pullViper, "enable_resource", PullCmd.Flags().Lookup("enable-resource"))
	PullCmd.Flags().StringP("text-line-break", "", "", "text file line break, default as LF")
	mustBindPFlag(pullViper, "text_line_break", PullCmd.Flags().Lookup("text-line-break"))

	for key, envName := range commonEnvs {
		// bind env variable with viper
		if err := pullViper.BindEnv(key, envName); err != nil {
			panic(err)
		}
		// add env info for cmdline flags
		if f := PullCmd.Flags().Lookup(strings.ReplaceAll(key, "_", "-")); f != nil {
			f.Usage = fmt.Sprintf("%v [env %v]", f.Usage, envName)
		}
	}
}
