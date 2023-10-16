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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/TencentBlueKing/bscp-go/cli/config"
	"github.com/TencentBlueKing/bscp-go/cli/constant"
)

var (
	feedAddrs string
	bizID     uint32
	appName   string
	labelsStr string
	labels    map[string]string
	uid       string
	token     string
	tempDir   string
	validArgs []string
	conf      = new(config.ClientConfig)
	// flag values
	configPath string
)

var (
	// !important: promise of compatibility
	// priority: Config File -> Command Options -> Environment Variables -> Defaults

	// rootEnvs variable definition
	rootEnvs = map[string]string{
		"verbose": "verbosity",
	}

	// commonEnvs variable definition
	commonEnvs = map[string]string{
		"biz":        "biz",
		"app":        "app",
		"labels":     "labels",
		"feed_addrs": "feed-addrs",
		"token":      "token",
		"temp_dir":   "temp-dir",
	}

	envLabelsPrefix = "labels_"
)

// initArgs init the common args
func initArgs() error {

	if configPath != "" {
		if err := initFromConfig(); err != nil {
			return err
		}
	} else {
		if err := initFromCmdArgs(); err != nil {
			return err
		}
	}
	initLabelsFromEnv()
	return nil
}

func initFromConfig() error {
	fmt.Println("use config file: ", configPath)
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file failed, err: %s", err.Error())
	}
	if err := viper.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config file failed, err: %s", err.Error())
	}
	if err := conf.Validate(); err != nil {
		return fmt.Errorf("validate watch config failed, err: %s", err.Error())
	}
	conf.Log.TrySetDefault()
	return nil
}

func initFromCmdArgs() error {
	fmt.Println("use command line args or environment variables")
	if bizID == 0 {
		return fmt.Errorf("biz id must not be 0")
	}
	validArgs = append(validArgs, fmt.Sprintf("--biz=%d", bizID))

	if appName == "" {
		return fmt.Errorf("app must not be empty")
	}
	validArgs = append(validArgs, fmt.Sprintf("--app=%s", appName))

	// labels is optional, if labels is not set, instance would match default group's release
	if labelsStr != "" {
		if json.Unmarshal([]byte(labelsStr), &labels) != nil {
			return fmt.Errorf("labels is not a valid json string")
		}
		validArgs = append(validArgs, fmt.Sprintf("--labels=%s", labelsStr))
	}

	if feedAddrs == "" {
		return fmt.Errorf("feed server address must not be empty")
	}
	validArgs = append(validArgs, fmt.Sprintf("--feed-addrs=%s", feedAddrs))

	if token == "" {
		return fmt.Errorf("token must not be empty")
	}
	validArgs = append(validArgs, fmt.Sprintf("--token=%s", "***"))

	if tempDir == "" {
		tempDir = constant.DefaultTempDir
	}
	validArgs = append(validArgs, fmt.Sprintf("--temp-dir=%s", tempDir))

	fmt.Println("args:", strings.Join(validArgs, " "))

	// construct config
	conf.Biz = bizID
	conf.FeedAddrs = strings.Split(feedAddrs, ",")
	conf.Token = token
	conf.Labels = labels
	conf.UID = uid
	conf.TempDir = tempDir

	apps := []*config.AppConfig{}
	for _, app := range strings.Split(appName, ",") {
		apps = append(apps, &config.AppConfig{Name: strings.TrimSpace(app)})
	}
	conf.Apps = apps
	conf.Log.TrySetDefault()
	return nil
}

func initLabelsFromEnv() {
	labels := make(map[string]string)
	// get multi labels from environment variables
	envs := os.Environ()
	for _, env := range envs {
		kv := strings.Split(env, "=")
		k, v := kv[0], kv[1]
		if strings.HasPrefix(k, envLabelsPrefix) && strings.TrimPrefix(k, envLabelsPrefix) != "" {
			labels[strings.TrimPrefix(k, envLabelsPrefix)] = v
		}
	}
	for k, v := range labels {
		conf.Labels[k] = v
	}
}
