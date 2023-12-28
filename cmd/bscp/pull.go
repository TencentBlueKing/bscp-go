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
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/dal/table"
	"github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/constant"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/eventmeta"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/util"
	pkgutil "github.com/TencentBlueKing/bscp-go/internal/util"
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

	// 设置日志等级
	level := logger.GetLevelByName(logLevel)
	logger.SetLevel(level)

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
	)
	if err != nil {
		logger.Error("init client", logger.ErrAttr(err))
		os.Exit(1)
	}
	for _, app := range conf.Apps {
		opts := []client.AppOption{}
		opts = append(opts, client.WithAppKey("**"))
		opts = append(opts, client.WithAppLabels(app.Labels))
		opts = append(opts, client.WithAppUID(app.UID))
		if conf.TempDir != "" {
			tempDir = conf.TempDir
		}
		if err = pullAppFiles(bscp, tempDir, conf.Biz, app.Name, opts); err != nil {
			logger.Error("pull files failed", logger.ErrAttr(err))
			os.Exit(1)
		}
	}
}

func pullAppFiles(bscp client.Client, tempDir string, biz uint32, app string, opts []client.AppOption) error {
	// 1. prepare app workspace dir
	appDir := path.Join(tempDir, strconv.Itoa(int(biz)), app)
	if e := os.MkdirAll(appDir, os.ModePerm); e != nil {
		return e
	}
	release, err := bscp.PullFiles(app, opts...)
	if err != nil {
		return err
	}
	// 2. execute pre hook
	if release.PreHook != nil {
		if err := util.ExecuteHook(release.PreHook, table.PreHook, tempDir, biz, app); err != nil {
			return err
		}
	}
	// 3. download files and save to temp dir
	filesDir := path.Join(appDir, "files")
	if err := util.UpdateFiles(filesDir, release.FileItems); err != nil {
		return err
	}
	// 4. execute post hook
	if release.PostHook != nil {
		if err := util.ExecuteHook(release.PostHook, table.PostHook, tempDir, biz, app); err != nil {
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
