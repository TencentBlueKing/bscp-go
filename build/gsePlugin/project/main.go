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

	"github.com/hashicorp/go-version"
	"github.com/spf13/cobra"
)

const (
	// 按照规则, 前面2位是依赖的 GseAgent 版本号, 常量值
	gsePluginVersion = "1.0"
)

func main() {
	pluginVer, err := makeGsePluginVersion(os.Args[1])
	fmt.Fprint(os.Stdout, pluginVer)
	cobra.CheckErr(err)
}

// mustMakeGsePluginVersion 生成GsePlugin版本规则
// 和 bscp-go 的对应的规则为1.0.{major}{minor}.{patch}
func makeGsePluginVersion(ver string) (string, error) {
	v, err := version.NewSemver(ver)
	if err != nil {
		return "", err
	}

	parts := v.Segments()
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid version %s", ver)
	}

	major := parts[0]
	minor := parts[1]
	patch := parts[2]

	pluginVer := fmt.Sprintf("%s.%d%02d.%d", gsePluginVersion, major, minor, patch)
	return pluginVer, nil
}
