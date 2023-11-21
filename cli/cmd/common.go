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

// Package cmd define the command line for bscp client.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"bscp.io/pkg/logs"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/TencentBlueKing/bscp-go/cli/config"
	"github.com/TencentBlueKing/bscp-go/cli/constant"
	"github.com/TencentBlueKing/bscp-go/pkg/util"
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

	envLabelsPrefix = "labels_"
)

// ReloadMessage reload message with event and error
type ReloadMessage struct {
	Event  fsnotify.Event
	Labels map[string]string
	Error  error
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
		// labels_file is a special env used to set labels file to watch
		// TODO: set envLabelsPrefix to 'label_' so that env key would not conflict with labels_file
		if k == "labels_file" {
			continue
		}
		if strings.HasPrefix(k, envLabelsPrefix) && strings.TrimPrefix(k, envLabelsPrefix) != "" {
			labels[strings.TrimPrefix(k, envLabelsPrefix)] = v
		}
	}
	conf.Labels = util.MergeLabels(conf.Labels, labels)
}

func watchLabelsFile(ctx context.Context, path string, oldLabels map[string]string) (chan ReloadMessage, error) {
	watchChan := make(chan ReloadMessage)
	v := viper.New()
	v.SetConfigFile(path)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("new watcher failed, err: %s", err.Error())
	}
	if err := watcher.Add(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("add watcher for %s failed, err: %s", path, err.Error())
	}
	go func() {
		oldLabels := oldLabels
		for {
			select {
			case <-ctx.Done():
				logs.Infof("watch labels file %s stoped because of %s", path, ctx.Err().Error())
				if err := watcher.Close(); err != nil {
					logs.Warnf("close watcher failed, err: %s", err.Error())
				}
				return
			case event := <-watcher.Events:
				msg := ReloadMessage{Event: event}
				if event.Op != fsnotify.Write {
					continue
				}

				absPath, err := filepath.Abs(event.Name)
				if err != nil {
					logs.Warnf("get labels file absPath failed, err: %s", err.Error())
					continue
				}
				if absPath != path {
					continue
				}

				if err := v.ReadInConfig(); err != nil {
					msg.Error = fmt.Errorf("read labels file failed, err: %s", err.Error())
					watchChan <- msg
					continue
				}

				labels := make(map[string]string)
				if err := v.Unmarshal(&labels); err != nil {
					msg.Error = fmt.Errorf("unmarshal labels file failed, err: %s", err.Error())
					watchChan <- msg
					continue
				}

				if reflect.DeepEqual(labels, oldLabels) {
					continue
				}

				logs.Infof("labels file %s changed, try reset labels, old: %s, new: %s", path, oldLabels, labels)
				msg.Labels = labels
				watchChan <- msg
				oldLabels = labels
			case err := <-watcher.Errors:
				logs.Errorf("watcher error: %s", err.Error())
			}
		}
	}()
	return watchChan, nil
}

func readLabelsFile(path string) (map[string]string, error) {
	v := viper.New()
	v.SetConfigFile(path)
	labels := make(map[string]string)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			logs.Warnf("labels file %s not exist, skip read", path)
			return labels, nil
		}
		return nil, fmt.Errorf("stat labels file %s failed, err: %s", path, err.Error())
	}
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read labels file %s failed, err: %s", path, err.Error())
	}
	if err := v.Unmarshal(&labels); err != nil {
		return nil, fmt.Errorf("unmarshal labels file %s failed, err: %s", path, err.Error())
	}
	return labels, nil
}
