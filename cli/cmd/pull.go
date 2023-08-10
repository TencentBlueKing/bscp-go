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
	"strings"
	"time"

	"bscp.io/pkg/dal/table"
	"bscp.io/pkg/logs"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bscp-go/cli/util"
	"github.com/TencentBlueKing/bscp-go/client"
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
	if err := validateArgs(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	fmt.Println("args:", strings.Join(validArgs, " "))

	opts := []client.Option{}
	opts = append(opts, client.FeedAddrs(strings.Split(feedAddrs, ",")))
	opts = append(opts, client.BizID(bizID))
	opts = append(opts, client.Labels(labels))
	opts = append(opts, client.Token(token))
	opts = append(opts, client.LogVerbosity(logVerbosity))

	if uid != "" {
		opts = append(opts, client.UID(uid))
	}
	bscp, err := client.New(opts...)
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	// 1. prepare app workspace dir
	if e := os.MkdirAll(tempDir, os.ModePerm); e != nil {
		logs.Errorf(e.Error())
		os.Exit(1)
	}
	releaseID, files, preHook, postHook, err := bscp.PullFiles(appName, "**")
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	// 2. execute pre hook
	if preHook != nil {
		if err := util.ExecuteHook(tempDir, preHook, table.PreHook); err != nil {
			logs.Errorf(err.Error())
			os.Exit(1)
		}
	}
	// 3. download files and save to temp dir
	filesDir := path.Join(tempDir, "files")
	if err := util.UpdateFiles(filesDir, files); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	// 4. execute post hook
	if postHook != nil {
		if err := util.ExecuteHook(tempDir, postHook, table.PostHook); err != nil {
			logs.Errorf(err.Error())
			os.Exit(1)
		}
	}
	// 5. append metadata to metadata.json
	metadata := &eventmeta.EventMeta{
		ReleaseID: releaseID,
		Status:    eventmeta.EventStatusSuccess,
		EventTime: time.Now().Format(time.RFC3339),
	}
	if err := eventmeta.AppendMetadataToFile(tempDir, metadata); err != nil {
		logs.Errorf("append metadata to file failed, err: %s", err.Error())
		os.Exit(1)
	}
}

func init() {
	// important: promise of compatibility
	PullCmd.Flags().SortFlags = false

	PullCmd.Flags().Uint32VarP(&bizID, "biz", "b", 0, "biz id")
	PullCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	PullCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	PullCmd.Flags().StringVarP(&uid, "uid", "u", "", "uid")
	PullCmd.Flags().StringVarP(&feedAddrs, "feed-addrs", "f", "", "feed server address, eg: 'bscp.io:8080,bscp.io:8081'")
	PullCmd.Flags().StringVarP(&token, "token", "t", "", "sdk token")
	PullCmd.Flags().StringVarP(&tempDir, "temp-dir", "c", "",
		"app config file temp dir, default: '/data/bscp/{biz_id}/{app_name}")

	for env, flag := range commonEnvs {
		flag := PullCmd.Flags().Lookup(flag)
		flag.Usage = fmt.Sprintf("%v [env %v]", flag.Usage, env)
		if value := os.Getenv(env); value != "" {
			flag.Value.Set(value)
		}
	}
}
