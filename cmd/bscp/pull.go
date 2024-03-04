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
	"path"
	"strconv"
	"strings"
	"sync/atomic"

	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/constant"
	"github.com/TencentBlueKing/bscp-go/internal/util"
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

	if err := initArgs(); err != nil {
		logger.Error("init", logger.ErrAttr(err))
		os.Exit(1)
	}

	if conf.LabelsFile != "" {
		labels, err := readLabelsFile(conf.LabelsFile)
		if err != nil {
			logger.Error("read labels file failed", logger.ErrAttr(err))
			os.Exit(1)
		}
		conf.Labels = util.MergeLabels(conf.Labels, labels)
	}
	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(conf.Labels),
		client.WithUID(conf.UID),
		client.WithFileCache(client.FileCache{
			Enabled:     *conf.FileCache.Enabled,
			CacheDir:    conf.FileCache.CacheDir,
			ThresholdGB: conf.FileCache.ThresholdGB,
		}),
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 是否采集/监控资源使用率
	if conf.EnableMonitorResourceUsage {
		go util.MonitorCPUAndMemUsage(ctx)
	}
	for _, app := range conf.Apps {
		opts := []client.AppOption{}
		opts = append(opts, client.WithAppKey("**"))
		opts = append(opts, client.WithAppLabels(app.Labels))
		opts = append(opts, client.WithAppUID(app.UID))
		if conf.TempDir != "" {
			tempDir = conf.TempDir
		}
		if err = pullAppFiles(ctx, bscp, tempDir, conf.Biz, app.Name, opts); err != nil {
			cancel()
			logger.Error("pull files failed", logger.ErrAttr(err))
			os.Exit(1)
		}
	}
	cancel()
}

func pullAppFiles(ctx context.Context, bscp client.Client, tempDir string, biz uint32, app string, opts []client.AppOption) error { // nolint

	// 1. prepare app workspace dir
	appDir := path.Join(tempDir, strconv.Itoa(int(biz)), app)
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

	PullCmd.Flags().StringVarP(&feedAddrs, "feed-addrs", "f", "",
		"feed server address, eg: 'bscp-feed.example.com:9510'")
	PullCmd.Flags().IntVarP(&bizID, "biz", "b", 0, "biz id")
	PullCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	PullCmd.Flags().StringVarP(&token, "token", "t", "", "sdk token")
	PullCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	PullCmd.Flags().StringVarP(&labelsFilePath, "labels-file", "", "", "labels file path")
	// TODO: set client UID
	PullCmd.Flags().StringVarP(&tempDir, "temp-dir", "d", "",
		fmt.Sprintf("bscp temp dir, default: '%s'", constant.DefaultTempDir))
	PullCmd.Flags().BoolVarP(fileCache.Enabled, "file-cache-enabled", "",
		constant.DefaultFileCacheEnabled, "enable file cache or not")
	PullCmd.Flags().StringVarP(&fileCache.CacheDir, "file-cache-dir", "",
		constant.DefaultFileCacheDir, "bscp file cache dir")
	PullCmd.Flags().Float64VarP(&fileCache.ThresholdGB, "cache-threshold-gb", "",
		constant.DefaultCacheThresholdGB, "bscp file cache threshold gigabyte")
	PullCmd.Flags().BoolVarP(&enableMonitorResourceUsage, "enable-resource", "e", true, "enable report resource usage")

	for env, f := range commonEnvs {
		flag := PullCmd.Flags().Lookup(f)
		flag.Usage = fmt.Sprintf("%v [env %v]", flag.Usage, env)
		if value := os.Getenv(env); value != "" {
			if err := flag.Value.Set(value); err != nil {
				panic(err)
			}
		}
	}
}
