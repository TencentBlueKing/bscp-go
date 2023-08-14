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

	"bscp.io/pkg/logs"
	"github.com/spf13/cobra"
)

var (
	logVerbosity uint
	rootCmd      = &cobra.Command{
		Use:   "bscp",
		Short: "bscp is a command line tool for blueking service config platform",
		Long:  `bscp is a command line tool for blueking service config platform`,
	}
)

// Execute executes the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
}

func init() {
	// 不开启 自动排序
	cobra.EnableCommandSorting = false
	// 不开启 completion 子命令
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.AddCommand(PullCmd)
	rootCmd.AddCommand(WatchCmd)
	rootCmd.AddCommand(VersionCmd)
	rootCmd.PersistentFlags().UintVarP(&logVerbosity, "verbosity", "v", 0, "log verbosity")

	for env, f := range rootEnvs {
		flag := rootCmd.PersistentFlags().Lookup(f)
		flag.Usage = fmt.Sprintf("%v [env %v]", flag.Usage, env)
		if value := os.Getenv(env); value != "" {
			if err := flag.Value.Set(value); err != nil {
				panic(err)
			}
		}
	}
}
