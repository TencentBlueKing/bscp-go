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
	"time"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/dal/table"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/constant"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/eventmeta"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/util"
	pkgutil "github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/internal/util/host"
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
		conf.Labels = pkgutil.MergeLabels(conf.Labels, labels)
	}
	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithLabels(conf.Labels),
		client.WithUID(conf.UID),
		client.WithClientMode(sfs.Pull),
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}
	// Determine whether to collect resources
	if conf.EnableReportResourceUsage {
		go host.MonitorCPUAndMemUsage()
	}

	bscp.NewHeartbeat()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

}

func pullAppFiles(ctx context.Context, bscp client.Client, tempDir string, biz uint32, app string, opts []client.AppOption) error { // nolint
	startTime := time.Now()
	var (
		failedReason       sfs.FailedReason
		status             sfs.Status
		totalFileNum       int
		failedDetailReason string
		targetReleaseID    uint32
		totalFileSize      uint64
	)
	status = sfs.Processing
	application := bscp.GetApplication()

	options := &client.AppOptions{}
	for _, opt := range opts {
		opt(options)
	}

	meta := sfs.SideAppMeta{
		App:                 app,
		Uid:                 options.UID,
		Labels:              options.Labels,
		CursorID:            application.CursorID,
		ReleaseChangeStatus: status,
	}

	application.App = app
	application.ReleaseChangeStatus = status

	cpuUsage, cpuMaxUsage := host.GetCpuUsage()
	memoryUsage, memoryMaxUsage := host.GetMemUsage()

	defer func() {
		// 处理变更事件
		messaging := client.VersionChangeMessaging{}
		messaging.EndTime = time.Now()
		messaging.StartTime = startTime
		messaging.FailedDetailReason = failedDetailReason
		messaging.Reason = failedReason
		messaging.Resource = sfs.ResourceUsage{
			CpuMaxUsage:    cpuMaxUsage,
			CpuUsage:       cpuUsage,
			MemoryMaxUsage: memoryMaxUsage,
			MemoryUsage:    memoryUsage,
		}
		messaging.Meta = meta
		messaging.Meta.TargetReleaseID = targetReleaseID
		messaging.Meta.DownloadFileNum = application.DownloadFileNum
		messaging.Meta.DownloadFileSize = application.DownloadFileSize
		messaging.Meta.ReleaseChangeStatus = status
		if errM := bscp.SendVersionChangeMessaging(messaging); errM != nil {
			logger.Error("description failed to report the client change event, client_mode: %s, biz: %d,app: %s, err: %s",
				sfs.Pull.String(), biz, app, errM.Error())
		}
		// 发送变更后的pull事件
		err := bscp.SendPullStatusMessaging(messaging.Meta, startTime, status, failedReason, failedDetailReason)
		if err != nil {
			logger.Error("failed to send the pull status event. biz: %d,app: %s, err: %s",
				biz, app, err.Error())
		}
	}()

	// 1. prepare app workspace dir
	appDir := path.Join(tempDir, strconv.Itoa(int(biz)), app)
	if e := os.MkdirAll(appDir, os.ModePerm); e != nil {
		failedReason, status, failedDetailReason = sfs.DownloadFailed, sfs.Failed, e.Error()
		return e
	}

	release, err := bscp.PullFiles(app, opts...)
	if err != nil {
		failedReason, status, failedDetailReason = sfs.DownloadFailed, sfs.Failed, err.Error()
		return err
	}
	release.SemaphoreCh = make(chan struct{})
	// 计算文件数
	totalFileNum = len(release.FileItems)
	targetReleaseID = release.ReleaseID
	for _, item := range release.FileItems {
		totalFileSize += item.FileMeta.ContentSpec.GetByteSize()
	}

	// 拉取前上报拉取事件
	meta.TargetReleaseID = targetReleaseID
	meta.TotalFileNum = totalFileNum
	meta.TotalFileSize = totalFileSize
	err = bscp.SendPullStatusMessaging(meta, startTime, status, failedReason, "")
	if err != nil {
		logger.Error("failed to send the pull status event. biz: %d,app: %s, err: %s",
			biz, app, err.Error())
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-release.SemaphoreCh:
				successDownloads := atomic.LoadInt32(&release.DownloadFileNum)
				successFileSize := atomic.LoadUint64(&release.DownloadFileSize)
				application.DownloadFileNum = successDownloads
				application.DownloadFileSize = successFileSize
			}
		}
	}()

	// 2. execute pre hook
	if release.PreHook != nil {
		if errEH := util.ExecuteHook(release.PreHook, table.PreHook, tempDir, biz, app); errEH != nil {
			failedReason, status, failedDetailReason = sfs.PreHookFailed, sfs.Failed, errEH.Error()
			return errEH
		}
	}
	// 3. download files and save to temp dir
	filesDir := path.Join(appDir, "files")
	err = util.UpdateFiles(filesDir, release.FileItems, &release.DownloadFileNum, &release.DownloadFileSize,
		release.SemaphoreCh)
	if err != nil {
		failedReason, status, failedDetailReason = sfs.DownloadFailed, sfs.Failed, err.Error()
		return err
	}

	// 4. execute post hook
	if release.PostHook != nil {
		if err := util.ExecuteHook(release.PostHook, table.PostHook, tempDir, biz, app); err != nil {
			failedReason, status, failedDetailReason = sfs.PostHookFailed, sfs.Failed, err.Error()
			return err
		}
	}
	// 5. append metadata to metadata.json
	metadata := &eventmeta.EventMeta{
		ReleaseID: release.ReleaseID,
		Status:    eventmeta.EventStatusSuccess,
		EventTime: time.Now().Format(time.RFC3339),
	}
	if err := eventmeta.AppendMetadataToFile(appDir, metadata); err != nil {
		failedReason, status, failedDetailReason = sfs.DownloadFailed, sfs.Failed, err.Error()
		return err
	}
	status = sfs.Success
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
	PullCmd.Flags().BoolVarP(&enableReportResourceUsage, "enable-resource", "e", true, "enable report resource usage")

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
