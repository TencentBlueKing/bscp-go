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
 *
 */

package cmd

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	"bscp.io/pkg/dal/table"
	"bscp.io/pkg/logs"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bscp-go/cli/constant"
	"github.com/TencentBlueKing/bscp-go/cli/util"
	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/option"
	"github.com/TencentBlueKing/bscp-go/pkg/eventmeta"
)

var (
	PullCmd = &cobra.Command{
		Use:   "pull",
		Short: "pull",
		Long:  `pull `,
		Run:   Pull,
	}
)

// Pull executes the pull command.
func Pull(cmd *cobra.Command, args []string) {
	if err := initArgs(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}

	bscp, err := client.New(
		option.FeedAddrs(conf.FeedAddrs),
		option.BizID(conf.Biz),
		option.Token(conf.Token),
		option.Labels(conf.Labels),
		option.UID(conf.UID),
		option.LogVerbosity(logVerbosity),
	)
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	for _, app := range conf.Apps {
		opts := []option.AppOption{}
		opts = append(opts, option.WithKey("**"))
		opts = append(opts, option.WithLabels(app.Labels))
		opts = append(opts, option.WithUID(app.UID))
		if conf.TempDir != "" {
			tempDir = conf.TempDir
		}
		if err = pullAppFiles(bscp, tempDir, conf.Biz, app.Name, opts); err != nil {
			logs.Errorf(err.Error())
			os.Exit(1)
		}
	}
}

func pullAppFiles(bscp *client.Client, tempDir string, biz uint32, app string, opts []option.AppOption) error {
	// 1. prepare app workspace dir
	appDir := path.Join(tempDir, strconv.Itoa(int(biz)), app)
	if e := os.MkdirAll(appDir, os.ModePerm); e != nil {
		return e
	}
	releaseID, files, preHook, postHook, err := bscp.PullFiles(app, opts...)
	if err != nil {
		return err
	}
	// 2. execute pre hook
	if preHook != nil {
		if err := util.ExecuteHook(appDir, preHook, table.PreHook); err != nil {
			return err
		}
	}
	// 3. download files and save to temp dir
	filesDir := path.Join(appDir, "files")
	if err := util.UpdateFiles(filesDir, files); err != nil {
		return err
	}
	// 4. execute post hook
	if postHook != nil {
		if err := util.ExecuteHook(appDir, postHook, table.PostHook); err != nil {
			return err
		}
	}
	// 5. append metadata to metadata.json
	metadata := &eventmeta.EventMeta{
		ReleaseID: releaseID,
		Status:    eventmeta.EventStatusSuccess,
		EventTime: time.Now().Format(time.RFC3339),
	}
	if err := eventmeta.AppendMetadataToFile(appDir, metadata); err != nil {
		return err
	}
	logs.Infof("pull files success, current releaseID: %d", releaseID)
	return nil
}

func init() {
	// !important: promise of compatibility
	PullCmd.Flags().SortFlags = false

	PullCmd.Flags().StringVarP(&feedAddrs, "feed-addrs", "f", "",
		"feed server address, eg: 'bscp.io:8080,bscp.io:8081'")
	PullCmd.Flags().Uint32VarP(&bizID, "biz", "b", 0, "biz id")
	PullCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	PullCmd.Flags().StringVarP(&token, "token", "t", "", "sdk token")
	PullCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	// TODO: set client UID
	PullCmd.Flags().StringVarP(&tempDir, "temp-dir", "d", "",
		fmt.Sprintf("bscp temp dir, default: '%s'", constant.DefaultTempDir))
	PullCmd.Flags().StringVarP(&configPath, "config", "c", "", "config file path")

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
