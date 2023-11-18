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

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bscp.io/pkg/dal/table"
	"bscp.io/pkg/logs"
	pbhook "bscp.io/pkg/protocol/core/hook"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bscp-go/cli/constant"
	"github.com/TencentBlueKing/bscp-go/cli/eventmeta"
	"github.com/TencentBlueKing/bscp-go/cli/util"
	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/metrics"
	"github.com/TencentBlueKing/bscp-go/option"
	pkgutil "github.com/TencentBlueKing/bscp-go/pkg/util"
	"github.com/TencentBlueKing/bscp-go/types"
)

var (
	// WatchCmd command to watch app files
	WatchCmd = &cobra.Command{
		Use:   "watch",
		Short: "watch",
		Long:  `watch `,
		Run:   Watch,
	}
)

// Watch run as a daemon to watch the config changes.
func Watch(cmd *cobra.Command, args []string) {
	if err := initArgs(); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}

	labels := conf.Labels
	var (
		labelsFromFile map[string]string
		reloadChan     chan ReloadMessage
		err            error
		absPath        string
	)

	if conf.LabelsFile != "" {
		absPath, err = filepath.Abs(conf.LabelsFile)
		if err != nil {
			logs.Errorf("read labels file path failed, err: %s", err.Error())
			os.Exit(1) //nolint:gocritic
		}

		labelsFromFile, err = readLabelsFile(absPath)
		if err != nil {
			logs.Errorf("read labels file failed, err: %s", err.Error())
			os.Exit(1) //nolint:gocritic
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reloadChan, err = watchLabelsFile(ctx, absPath)
		if err != nil {
			logs.Errorf("watch labels file failed, err: %s", err.Error())
			os.Exit(1) //nolint:gocritic
		}

		labels = pkgutil.MergeLabels(labels, labelsFromFile)
		logs.Infof("watching labels file: %s", absPath)
	}

	bscp, err := client.New(
		option.FeedAddrs(conf.FeedAddrs),
		option.BizID(conf.Biz),
		option.Token(conf.Token),
		option.Labels(labels),
		option.UID(conf.UID),
		option.LogVerbosity(logVerbosity),
	)
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
	for _, subscriber := range conf.Apps {
		if conf.TempDir != "" {
			tempDir = conf.TempDir
		}
		handler := &WatchHandler{
			Biz:        conf.Biz,
			App:        subscriber.Name,
			Labels:     subscriber.Labels,
			UID:        subscriber.UID,
			Lock:       sync.Mutex{},
			TempDir:    tempDir,
			AppTempDir: path.Join(tempDir, strconv.Itoa(int(conf.Biz)), subscriber.Name),
		}
		if err := bscp.AddWatcher(handler.watchCallback, handler.App, handler.getSubscribeOptions()...); err != nil {
			logs.Errorf(err.Error())
			os.Exit(1)
		}
	}
	if e := bscp.StartWatch(); e != nil {
		logs.Errorf(e.Error())
		os.Exit(1)
	}

	go func() {
		if reloadChan == nil {
			return
		}
		for {
			msg := <-reloadChan
			if msg.Error != nil {
				logs.Errorf("reload labels failed, err: %s", msg.Error.Error())
				continue
			}
			bscp.ResetLabels(pkgutil.MergeLabels(conf.Labels, msg.Labels))
			logs.Infof("reload labels success")
		}
	}()

	// register metrics
	metrics.RegisterMetrics()
	http.Handle("/metrics", promhttp.Handler())
	if e := http.ListenAndServe(fmt.Sprintf(":%d", conf.Port), nil); e != nil {
		logs.Errorf("start http server failed, err: %s", e.Error())
		os.Exit(1)
	}
}

// WatchHandler watch handler
type WatchHandler struct {
	// Biz BSCP biz id
	Biz uint32
	// App BSCP app name
	App string
	// Labels instance labels
	Labels map[string]string
	// UID instance unique uid
	UID string
	// TempDir bscp temporary directory
	TempDir string
	// AppTempDir app temporary directory
	AppTempDir string
	// Lock lock for concurrent callback
	Lock sync.Mutex
}

func (w *WatchHandler) watchCallback(releaseID uint32, files []*types.ConfigItemFile,
	preHook *pbhook.HookSpec, postHook *pbhook.HookSpec) error {
	w.Lock.Lock()
	defer w.Lock.Unlock()

	lastMetadata, err := eventmeta.GetLatestMetadataFromFile(w.AppTempDir)
	if err != nil {
		logs.Warnf("get latest release metadata failed, err: %s, maybe you should exec pull command first", err.Error())
	} else if lastMetadata.ReleaseID == releaseID {
		logs.Infof("current release is consistent with the received release %d, skip", releaseID)
		return nil
	}

	// 1. execute pre hook
	if preHook != nil {
		if err := util.ExecuteHook(preHook, table.PreHook, w.TempDir, w.Biz, w.App); err != nil {
			logs.Errorf(err.Error())
			return err
		}
	}

	filesDir := path.Join(w.AppTempDir, "files")
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
		if err := util.ExecuteHook(postHook, table.PostHook, w.TempDir, w.Biz, w.App); err != nil {
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
	if err := eventmeta.AppendMetadataToFile(w.AppTempDir, metadata); err != nil {
		logs.Errorf("append metadata to file failed, err: %s", err.Error())
		return err
	}
	// TODO: 6.2 call the callback notify api
	logs.Infof("watch release change success, current releaseID: %d", releaseID)
	return nil
}

func (w *WatchHandler) getSubscribeOptions() []option.AppOption {
	options := []option.AppOption{}
	options = append(options, option.WithLabels(w.Labels))
	options = append(options, option.WithUID(w.UID))
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
				if strings.HasPrefix(absFileDir, filePath) {
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

func init() {
	// !important: promise of compatibility
	WatchCmd.Flags().SortFlags = false

	WatchCmd.Flags().StringVarP(&feedAddrs, "feed-addrs", "f", "",
		"feed server address, eg: 'bscp.io:8080,bscp.io:8081'")
	WatchCmd.Flags().IntVarP(&bizID, "biz", "b", 0, "biz id")
	WatchCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	WatchCmd.Flags().StringVarP(&token, "token", "t", "", "sdk token")
	WatchCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	WatchCmd.Flags().StringVarP(&labelsFilePath, "labels-file", "", "", "labels file path")
	// TODO: set client UID
	WatchCmd.Flags().StringVarP(&tempDir, "temp-dir", "d", "",
		fmt.Sprintf("bscp temp dir, default: '%s'", constant.DefaultTempDir))
	WatchCmd.Flags().IntVarP(&port, "port", "p", constant.DefaultHttpPort, "sidecar http port")

	envs := map[string]string{}
	for env, f := range commonEnvs {
		envs[env] = f
	}
	for env, f := range watchEnvs {
		envs[env] = f
	}

	for env, f := range envs {
		flag := WatchCmd.Flags().Lookup(f)
		flag.Usage = fmt.Sprintf("%v [env %v]", flag.Usage, env)
		if value := os.Getenv(env); value != "" {
			if err := flag.Value.Set(value); err != nil {
				panic(err)
			}
		}
	}
}
