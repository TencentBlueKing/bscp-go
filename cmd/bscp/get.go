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
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	outputFormat string
	key          string
	match        []string
)

var (
	getCmd = &cobra.Command{
		Use:   "get",
		Short: "Display app or kv resources",
		Long:  `Display app or kv resources`,
	}

	getAppCmd = &cobra.Command{
		Use:   "app",
		Short: "Display app resources",
		Long:  `Display app resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetApp()
		},
	}

	getKvCmd = &cobra.Command{
		Use:   "kv",
		Short: "Display kv resources",
		Long:  `Display kv resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetKv()
		},
	}
)

func init() {
	// 公共参数
	getCmd.PersistentFlags().StringVarP(&feedAddrs, "feed-addrs", "f", "",
		"feed server address, eg: 'bscp-feed.example.com:9510'")
	getCmd.PersistentFlags().IntVarP(&bizID, "biz", "b", 0, "biz id")
	getCmd.PersistentFlags().StringVarP(&token, "token", "t", "", "sdk token")
	getCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "output format, current support: json")
	getCmd.PersistentFlags().StringArrayVarP(&match, "match", "m", []string{}, "match primary name pattern")

	// kv 参数
	getKvCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	getKvCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	getKvCmd.Flags().StringVarP(&key, "key", "k", "", "get kv raw value")
}

// runGetApp executes the get app command.
func runGetApp() error {
	logger.SetLevel(slog.LevelError)

	bscp, err := client.New(
		client.WithFeedAddrs(strings.Split(feedAddrs, ",")),
		client.WithBizID(uint32(bizID)),
		client.WithToken(token),
	)

	if err != nil {
		return err
	}

	apps, err := bscp.ListApps(match)
	if err != nil {
		return err
	}

	tableOutput := func() error {
		table := newTable()
		table.SetHeader([]string{"Name", "Config_Type", "Reviser", "UpdateAt"})
		for _, v := range apps {
			table.Append([]string{
				v.Name,
				v.ConfigType,
				v.Reviser,
				refineOutputTime(v.UpdateAt),
			})
		}
		table.Render()

		return nil
	}

	switch outputFormat {
	case "json":
		return jsonOutput(apps)
	case "":
		return tableOutput()
	default:
		return fmt.Errorf(
			`unable to match a printer suitable for the output format "%s", allowed formats are: json`, outputFormat)

	}
}

func runGetListKv(bscp client.Client, app string, match []string) error {
	release, err := bscp.PullKvs(app, match)
	if err != nil {
		return err
	}

	tableOutput := func() error {
		table := newTable()
		table.SetHeader([]string{"Key", "Type", "Reviser", "UpdateAt"})

		for _, v := range release.KvItems {
			table.Append([]string{
				v.Key,
				v.KvType,
				v.Reviser,
				refineOutputTime(v.UpdateAt),
			})
		}

		table.Render()
		return nil
	}

	switch outputFormat {
	case "json":
		return jsonOutput(release.KvItems)
	case "":
		return tableOutput()
	default:
		return fmt.Errorf(
			`unable to match a printer suitable for the output format "%s", allowed formats are: json`, outputFormat)
	}
}

func runGetKvValue(bscp client.Client, app, key string) error {
	value, err := bscp.Get(app, key)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(os.Stdout, value)
	return err
}

// runGetKv executes the get kv command.
func runGetKv() error {
	logger.SetLevel(slog.LevelError)

	bscp, err := client.New(
		client.WithFeedAddrs(strings.Split(feedAddrs, ",")),
		client.WithBizID(uint32(bizID)),
		client.WithToken(token),
	)

	if err != nil {
		return err
	}

	if key != "" {
		return runGetKvValue(bscp, appName, key)
	}

	return runGetListKv(bscp, appName, match)
}
