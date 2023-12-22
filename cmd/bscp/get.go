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
	"time"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var (
	// GetCmd command to pull app files
	getCmd = &cobra.Command{
		Use:   "get",
		Short: "display app or kv resources",
		Long:  `get `,
		Run:   runGet,
	}

	getAppCmd = &cobra.Command{
		Use:   "app",
		Short: "display app resources",
		Long:  `get `,
		Run:   runGetApp,
	}
)

// runGet executes the get command.
func runGet(cmd *cobra.Command, args []string) {
	fmt.Println("leji")
}

func runGetApp(cmd *cobra.Command, args []string) {
	bscp, err := client.New(
		client.WithFeedAddrs([]string{"127.0.0.1:9510"}),
		client.WithBizID(2),
		client.WithToken(""),
	)

	if err != nil {
		logger.Error("list app failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	apps, err := bscp.ListApps()
	if err != nil {
		logger.Error("list app failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	table := newTable()
	table.SetHeader([]string{"Name", "Config_Type", "Reviser", "UpdateAt"})

	for _, v := range apps {
		t, err := time.Parse(time.RFC3339, v.UpdateAt)

		var durStr string
		if err != nil {
			durStr = "N/A"
		} else {
			durStr = humanize.Time(t)
		}

		table.Append([]string{
			v.Name,
			v.ConfigType,
			v.Reviser,
			durStr,
		})
	}
	table.Render()
}
