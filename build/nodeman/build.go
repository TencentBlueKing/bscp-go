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

// program nodeman defines the bscp node manager plugin main entry.
package main

import (
	"fmt"
	"io"
	"os"
	_ "unsafe"

	"github.com/Tencent/bk-bcs/bcs-common/common"
	"github.com/Tencent/bk-bcs/bcs-common/common/conf"
)

const (
	// pluginName represents the name of the plugin
	pluginName = "bkbscp"
	// configPath represents the path to the configuration files
	configPath = "../etc/"
	// pidPath represents the path to the process identification (PID) file
	pidPath = "/var/run/gse"
)

func main() {
	args := os.Args
	var isVersion bool

	for i := 1; i < len(os.Args); i++ {
		if args[i] == "version" {
			isVersion = true
			break
		}
	}

	if !isVersion {
		originalFile, err := os.Open(fmt.Sprintf("%s%s.conf", configPath, pluginName))
		if err != nil {
			fmt.Printf("config file not exists")
			os.Exit(1)
		}

		defer func() {
			if e := originalFile.Close(); e != nil {
				fmt.Printf("close original file fail : %v\n", e)
			}
		}()

		newFile, err := os.Create(fmt.Sprintf("%s%s.yaml", configPath, pluginName))
		if err != nil {
			fmt.Printf("create config file fail, err: %v\n", err)
			os.Exit(1) //nolint:gocritic
		}

		defer func() {
			if e := newFile.Close(); e != nil {
				fmt.Printf("close new file fail : %v\n", e)
			}
		}()

		_, err = io.Copy(newFile, originalFile)
		if err != nil {
			fmt.Printf("copy config file fail, err: %v\n", err)
			os.Exit(1)
		}
		err = newFile.Sync()
		if err != nil {
			fmt.Printf("sync config file fail, err: %v\n", err)
			os.Exit(1)
		}

		cfg := conf.ProcessConfig{
			PidDir: pidPath,
		}
		if err := common.SavePid(cfg); err != nil {
			fmt.Printf("fail to save pid: err:%v", err)
		}

		os.Args = append(os.Args, "watch")
		os.Args = append(os.Args, "-c")
		os.Args = append(os.Args, fmt.Sprintf("%s%s.yaml", configPath, pluginName))
	}

	execute()
}

func execute() {}
