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
	"errors"
	"fmt"
	"os"
	"strings"

	"bscp.io/pkg/logs"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/TencentBlueKing/bscp-go/cli/config"
	"github.com/TencentBlueKing/bscp-go/cli/constant"
)

var (
	feedAddrs      string
	bizID          int
	appName        string
	labelsStr      string
	labelsFilePath string
	labels         map[string]string
	uid            string
	token          string
	tempDir        string
	validArgs      []string
	conf           = new(config.ClientConfig)
	labelsFromFile = make(map[string]string)
	// flag values
	configPath string
	port       int
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
		"biz":         "biz",
		"app":         "app",
		"labels":      "labels",
		"labels_file": "labels-file",
		"feed_addrs":  "feed-addrs",
		"token":       "token",
		"temp_dir":    "temp-dir",
	}

	watchEnvs = map[string]string{
		"port": "port",
	}

	envLabelPrefix = "label_"
)

// ReloadMessage reload message with event and error
type ReloadMessage struct {
	Event fsnotify.Event
	Error error
}

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
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file failed, err: %s", err.Error())
	}
	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config file failed, err: %s", err.Error())
	}
	if err := conf.Validate(); err != nil {
		return fmt.Errorf("validate watch config failed, err: %s", err.Error())
	}
	return nil
}

func initFromCmdArgs() error {
	fmt.Println("use command line args or environment variables")
	if bizID <= 0 {
		return fmt.Errorf("biz id must be greater than 0")
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

	if labelsFilePath != "" {
		validArgs = append(validArgs, fmt.Sprintf("--labels-file=%s", labelsFilePath))
	}

	validArgs = append(validArgs, fmt.Sprintf("--port=%d", port))

	fmt.Println("args:", strings.Join(validArgs, " "))

	// construct config
	conf.Biz = uint32(bizID)
	conf.FeedAddrs = strings.Split(feedAddrs, ",")
	conf.Token = token
	conf.Labels = labels
	conf.UID = uid
	conf.TempDir = tempDir
	conf.LabelsFile = labelsFilePath
	conf.Port = port

	apps := []*config.AppConfig{}
	for _, app := range strings.Split(appName, ",") {
		apps = append(apps, &config.AppConfig{Name: strings.TrimSpace(app)})
	}
	conf.Apps = apps
	return nil
}

func initLabelsFromEnv() {
	labels := make(map[string]string)
	// get multi labels from environment variables
	envs := os.Environ()
	for _, env := range envs {
		kv := strings.Split(env, "=")
		k, v := kv[0], kv[1]
		if strings.HasPrefix(k, envLabelPrefix) && strings.TrimPrefix(k, envLabelPrefix) != "" {
			labels[strings.TrimPrefix(k, envLabelPrefix)] = v
		}
	}
	for k, v := range labels {
		conf.Labels[k] = v
	}
}

func watchLabelsFile(path string) (chan ReloadMessage, error) {
	watchChan := make(chan ReloadMessage)
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read labels file failed, err: %s", err.Error())
	}
	v.WatchConfig()
	if err := v.Unmarshal(&labelsFromFile); err != nil {
		return nil, fmt.Errorf("unmarshal labels file failed, err: %s", err.Error())
	}
	v.OnConfigChange(func(e fsnotify.Event) {
		msg := ReloadMessage{Event: e}
		if e.Op != fsnotify.Write {
			return
		}
		logs.Infof("labels file changed, reload labels")
		if err := v.ReadInConfig(); err != nil {
			logs.Infof("read labels file failed, err: ", err.Error())
			msg.Error = fmt.Errorf("read labels file failed, err: %s", err.Error())
			watchChan <- msg
			return
		}
		if err := v.Unmarshal(&labelsFromFile); err != nil {
			logs.Infof("unmarshal labels file failed, err: ", err.Error())
			msg.Error = errors.New("unmarshal labels file failed")
			watchChan <- msg
			return
		}
		watchChan <- msg
	})

	return watchChan, nil
}
