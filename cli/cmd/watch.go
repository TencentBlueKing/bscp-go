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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bscp.io/pkg/dal/table"
	"bscp.io/pkg/logs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	pbhook "bscp.io/pkg/protocol/core/hook"
	// for unmarshal yaml config file
	_ "gopkg.in/yaml.v2"

	"github.com/TencentBlueKing/bscp-go/cli/util"
	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/pkg/eventmeta"
	"github.com/TencentBlueKing/bscp-go/types"
	"github.com/TencentBlueKing/bscp-go/watch"
)

var (
	WatchCmd = &cobra.Command{
		Use:   "watch",
		Short: "watch",
		Long:  `watch `,
		Run:   Watch,
	}
	watchConfig = new(WatchConfig)
	// flag values
	watchConfigPath string
)

// WatchConfig config for bscp-go when run as daemon
type WatchConfig struct {
	// FeedAddrs bscp feed server addresses
	FeedAddrs []string `yaml:"feedAddrs"`
	// BizID bscp biz id
	BizID uint32 `yaml:"bizID"`
	// Token bscp sdk token
	Token string `yaml:"token"`
	// Apps bscp watched apps
	Apps []*AppConfig `yaml:"apps"`
}

// Validate validate the watch config
func (c *WatchConfig) Validate() error {
	if len(c.FeedAddrs) == 0 {
		return fmt.Errorf("feedAddrs is empty")
	}
	if c.BizID == 0 {
		return fmt.Errorf("bizID is empty")
	}
	if c.Token == "" {
		return fmt.Errorf("token is empty")
	}
	if len(c.Apps) == 0 {
		return fmt.Errorf("watched apps is empty")
	}
	exists := make(map[string]bool)
	for _, app := range c.Apps {
		if exists[app.App] {
			return fmt.Errorf("watch repeated for app %s: ", app.App)
		}
		if err := app.Validate(); err != nil {
			return err
		}
		exists[app.App] = true
	}
	return nil
}

// AppConfig config for watched app
type AppConfig struct {
	// App BSCP app name
	App string `yaml:"app"`
	// Labels instance labels
	Labels map[string]string `yaml:"labels"`
	// UID instance unique uid
	UID string `yaml:"uid"`
	// TempDir config files temporary directory
	TempDir string
}

// Validate validate the app watch config
func (c *AppConfig) Validate() error {
	if c.App == "" {
		return fmt.Errorf("app is empty")
	}
	if len(c.Labels) == 0 {
		return fmt.Errorf("labels is empty")
	}
	return nil
}

// Watch run as a daemon to watch the config changes.
func Watch(cmd *cobra.Command, args []string) {
	if err := validateWatch(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	var bscp *client.Client
	var err error
	if watchConfigPath != "" {
		bscp, err = client.New(
			client.FeedAddrs(watchConfig.FeedAddrs),
			client.BizID(watchConfig.BizID),
			client.LogVerbosity(logVerbosity),
			client.Token(watchConfig.Token),
		)
	} else {
		bscp, err = client.New(
			client.FeedAddrs(strings.Split(feedAddrs, ",")),
			client.BizID(bizID),
			client.LogVerbosity(logVerbosity),
			client.Token(token),
		)
	}
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	if watchConfigPath != "" {
		for _, subscriber := range watchConfig.Apps {
			handler := &WatchHandler{
				BizID:   watchConfig.BizID,
				App:     subscriber.App,
				Labels:  subscriber.Labels,
				UID:     subscriber.UID,
				TempDir: subscriber.TempDir,
				Lock:    sync.Mutex{},
			}
			if handler.TempDir == "" {
				handler.TempDir = fmt.Sprintf("/data/bscp/%d/%s", handler.BizID, handler.App)
			}
			if err := bscp.AddWatcher(handler.watchCallback, handler.getSubscribeOptions()...); err != nil {
				logs.Errorf(err.Error())
				os.Exit(1)
			}
		}
	} else {
		handler := &WatchHandler{
			BizID:   bizID,
			App:     appName,
			Labels:  labels,
			UID:     uid,
			TempDir: tempDir,
			Lock:    sync.Mutex{},
		}
		if handler.TempDir == "" {
			handler.TempDir = fmt.Sprintf("/data/bscp/%d/%s", handler.BizID, handler.App)
		}
		if err := bscp.AddWatcher(handler.watchCallback, handler.getSubscribeOptions()...); err != nil {
			logs.Errorf(err.Error())
			os.Exit(1)
		}
	}
	if _, err := bscp.StartWatch(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	time.Sleep(time.Hour * 24 * 365)
}

// WatchHandler watch handler
type WatchHandler struct {
	// BizID bscp biz id
	BizID uint32
	// App BSCP app name
	App string
	// Labels instance labels
	Labels map[string]string
	// UID instance unique uid
	UID string
	// TempDir config files temporary directory
	TempDir string
	// Lock lock for concurrent callback
	Lock sync.Mutex
}

func (w *WatchHandler) watchCallback(releaseID uint32, files []*types.ConfigItemFile,
	preHook *pbhook.HookSpec, postHook *pbhook.HookSpec) error {
	w.Lock.Lock()
	defer w.Lock.Unlock()

	lastMetadata, err := eventmeta.GetLatestMetadataFromFile(w.TempDir)
	if err != nil {
		logs.Warnf("get latest release metadata failed, err: %s, maybe you should exec pull command first", err.Error())
	} else {
		if lastMetadata.ReleaseID == releaseID {
			logs.Infof("current release is consistent with the received release %d, skip", releaseID)
			return nil
		}
	}

	// 1. execute pre hook
	if preHook != nil {
		if err := util.ExecuteHook(tempDir, preHook, table.PreHook); err != nil {
			logs.Errorf(err.Error())
			return err
		}
	}

	filesDir := path.Join(w.TempDir, "files")
	if err := util.UpdateFiles(filesDir, files); err != nil {
		logs.Errorf(err.Error())
		return err
	}
	// 4. clear old files
	if err := clearOldFiles(filesDir, files); err != nil {
		logs.Errorf("clear old files failed, err: %s", err.Error())
		return err
	}
	// 5. execute post hook
	if postHook != nil {
		if err := util.ExecuteHook(tempDir, postHook, table.PostHook); err != nil {
			logs.Errorf(err.Error())
			return err
		}
	}
	// 6. reload app
	// 6.1 append metadata to metadata.json
	metadata := &eventmeta.EventMeta{
		ReleaseID: releaseID,
		Status:    eventmeta.EventStatusSuccess,
		EventTime: time.Now().Format(time.RFC3339),
	}
	if err := eventmeta.AppendMetadataToFile(w.TempDir, metadata); err != nil {
		logs.Errorf("append metadata to file failed, err: %s", err.Error())
		return err
	}
	// TODO: 6.2 call the callback notify api
	return nil
}

func (w *WatchHandler) getSubscribeOptions() []watch.SubscribeOption {
	options := []watch.SubscribeOption{}
	options = append(options, watch.WithApp(w.App))
	options = append(options, watch.WithLabels(w.Labels))
	options = append(options, watch.WithUID(w.UID))
	return options
}

func clearOldFiles(dir string, files []*types.ConfigItemFile) error {
	err := filepath.Walk(dir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			for _, file := range files {
				absFileDir := filepath.Join(dir, file.Path)
				if filepath.HasPrefix(absFileDir, filePath) {
					return nil
				}
			}
			if err := os.RemoveAll(filePath); err != nil {
				return err
			}
			return filepath.SkipDir
		}

		for _, file := range files {
			absFile := filepath.Join(dir, file.Path, file.Name)
			if absFile == filePath {
				return nil
			}
		}
		return os.Remove(filePath)
	})

	return err
}

func validateWatch() error {
	if watchConfigPath != "" {
		fmt.Println("use watch config file: ", watchConfigPath)
		viper.SetConfigType("yaml")
		viper.SetConfigFile(watchConfigPath)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("read config file failed, err: %s", err.Error())
		}
		if err := viper.Unmarshal(watchConfig); err != nil {
			return fmt.Errorf("unmarshal config file failed, err: %s", err.Error())
		}
		if err := watchConfig.Validate(); err != nil {
			return fmt.Errorf("validate watch config failed, err: %s", err.Error())
		}
		return nil
	}

	fmt.Println("use watch command line args")
	if err := validateArgs(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}

	fmt.Println("args:", strings.Join(validArgs, " "))
	return nil
}

func init() {
	// important: promise of compatibility
	WatchCmd.Flags().SortFlags = false

	WatchCmd.Flags().Uint32VarP(&bizID, "biz", "b", 0, "biz id")
	WatchCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	WatchCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	WatchCmd.Flags().StringVarP(&uid, "uid", "u", "", "uid")
	WatchCmd.Flags().StringVarP(&feedAddrs, "feed-addrs", "f", "",
		"feed server address, eg: 'bscp.io:8080,bscp.io:8081'")
	WatchCmd.Flags().StringVarP(&token, "token", "t", "", "sdk token")
	WatchCmd.Flags().StringVarP(&tempDir, "temp-dir", "d", "",
		"app config file temp dir, default: '/data/bscp/{biz_id}/{app_name}")
	WatchCmd.Flags().StringVarP(&watchConfigPath, "config", "c", "", "watch config")

	for env, flag := range commonEnvs {
		flag := WatchCmd.Flags().Lookup(flag)
		flag.Usage = fmt.Sprintf("%v [env %v]", flag.Usage, env)
		if value := os.Getenv(env); value != "" {
			flag.Value.Set(value)
		}
	}
}
