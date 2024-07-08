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

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bscp-go/internal/constant"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	rootCmd = &cobra.Command{
		Use:   "bscp",
		Short: "bscp is a command line tool for blueking service config platform",
		Long:  `bscp is a command line tool for blueking service config platform`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 设置日志等级
			level := logger.GetLevelByName(rootViper.GetString("log_level"))
			logger.SetLevel(level)

			return nil
		},
	}
)

// Execute executes the root command.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	// 不开启 自动排序
	cobra.EnableCommandSorting = false
	// 不开启 completion 子命令
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	getCmd.AddCommand(getAppCmd)
	getCmd.AddCommand(getFileCmd)
	getCmd.AddCommand(getKvCmd)
	rootCmd.AddCommand(getCmd)

	rootCmd.AddCommand(PullCmd)
	rootCmd.AddCommand(WatchCmd)
	rootCmd.AddCommand(VersionCmd)
	rootCmd.PersistentFlags().StringP(
		"log_level", "", "", "log filtering level, One of: debug|info|warn|error. (default info)")
	rootCmd.PersistentFlags().StringP("config", "c", constant.DefaultConfFile, "config file path")
	cfgFlag := rootCmd.PersistentFlags().Lookup("config")
	logLevelFlag := rootCmd.PersistentFlags().Lookup("log_level")
	// add env info for cmdline flags
	cfgFlag.Usage = fmt.Sprintf("%v [env %v]", cfgFlag.Usage, rootEnvs["config_file"])
	logLevelFlag.Usage = fmt.Sprintf("%v [env %v]", logLevelFlag.Usage, rootEnvs["log_level"])

	for _, v := range allVipers {
		mustBindPFlag(v, "config_file", cfgFlag)
		mustBindPFlag(v, "log_level", logLevelFlag)

		for key, envName := range rootEnvs {
			// bind env variable with viper
			if err := v.BindEnv(key, envName); err != nil {
				panic(err)
			}
		}
	}
}
