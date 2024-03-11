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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/constant"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	outputFormat string
)

const (
	outputFormatTable     = ""
	outputFormatJson      = "json"
	outputFormatValue     = "value"
	outputFormatValueJson = "value_json"
	outputFormatContent   = "content"
)

var (
	getCmd = &cobra.Command{
		Use:   "get",
		Short: "Display app, file or kv resources",
		Long:  `Display app, file or kv resources`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 设置日志等级, get 命令默认是 error
			if logLevel == "" {
				logLevel = "error"
			}

			level := logger.GetLevelByName(logLevel)
			logger.SetLevel(level)

			// 校验&反序列化 labels
			if labelsStr != "" {
				if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
					return fmt.Errorf("invalid labels: %w", err)
				}
			}

			return nil
		},
	}

	getAppCmd = &cobra.Command{
		Use:   "app [res...]",
		Short: "Display app resources",
		Long:  `Display app resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetApp(args)
		},
	}

	getFileCmd = &cobra.Command{
		Use:   "file [res...]",
		Short: "Display file resources",
		Long:  `Display file resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetFile(args)
		},
	}

	getKvCmd = &cobra.Command{
		Use:   "kv [res...]",
		Short: "Display kv resources",
		Long:  `Display kv resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetKv(args)
		},
	}
)

func init() {
	// 公共参数
	getCmd.PersistentFlags().StringVarP(&feedAddrs, "feed-addrs", "f", "",
		"feed server address, eg: 'bscp-feed.example.com:9510'")
	getCmd.PersistentFlags().IntVarP(&bizID, "biz", "b", 0, "biz id")
	getCmd.PersistentFlags().StringVarP(&token, "token", "t", "", "sdk token")

	// app 参数
	getAppCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "output format, One of: json")

	// file 参数
	getFileCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	getFileCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	getFileCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "output format, One of: json|content")
	getFileCmd.Flags().BoolVarP(fileCache.Enabled, "file-cache-enabled", "",
		constant.DefaultFileCacheEnabled, "enable file cache or not")
	getFileCmd.Flags().StringVarP(&fileCache.CacheDir, "file-cache-dir", "",
		constant.DefaultFileCacheDir, "bscp file cache dir")
	getFileCmd.Flags().Float64VarP(&fileCache.ThresholdGB, "cache-threshold-gb", "",
		constant.DefaultCacheThresholdGB, "bscp file cache threshold gigabyte")

	// kv 参数
	getKvCmd.Flags().StringVarP(&appName, "app", "a", "", "app name")
	getKvCmd.Flags().StringVarP(&labelsStr, "labels", "l", "", "labels")
	getKvCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "output format, One of: json|value|value_json")
}

// runGetApp executes the get app command.
func runGetApp(args []string) error {
	baseConf, err := initBaseConf()
	if err != nil {
		return err
	}

	bscp, err := client.New(
		client.WithFeedAddrs(baseConf.GetFeedAddrs()),
		client.WithBizID(baseConf.Biz),
		client.WithToken(baseConf.Token),
	)

	if err != nil {
		return err
	}

	apps, err := bscp.ListApps(args)
	if err != nil {
		return err
	}

	tableOutput := func() error {
		table := newTable()
		table.SetHeader([]string{"Name", "Config_Type", "Reviser", "UpdateAt"})
		for _, v := range apps {
			table.Append([]string{
				v.Name,
				v.ConfigType,
				v.Revision.Reviser,
				refineOutputTime(v.Revision.UpdateAt),
			})
		}
		table.Render()

		return nil
	}

	switch outputFormat {
	case outputFormatJson:
		return jsonOutput(apps)
	case outputFormatTable:
		return tableOutput()
	default:
		return fmt.Errorf(
			`unable to match a printer suitable for the output format "%s", allowed formats are: json`, outputFormat)

	}
}

func runGetFileList(bscp client.Client, app string, match []string) error {
	var opts []client.AppOption
	if len(match) > 0 {
		opts = append(opts, client.WithAppKey(match[0]))
	}
	opts = append(opts, client.WithAppLabels(labels))

	release, err := bscp.PullFiles(app, opts...)
	if err != nil {
		return err
	}

	tableOutput := func() error {
		table := newTable()
		table.SetHeader([]string{"File", "ContentID", "Size", "Reviser", "UpdateAt"})
		for _, v := range release.FileItems {
			table.Append([]string{
				path.Join(v.Path, v.Name),
				v.FileMeta.ContentSpec.Signature,
				humanize.IBytes(v.FileMeta.ContentSpec.ByteSize),
				v.FileMeta.ConfigItemRevision.Reviser,
				refineOutputTime(v.FileMeta.ConfigItemRevision.UpdateAt),
			})
		}

		table.Render()
		return nil
	}

	switch outputFormat {
	case outputFormatJson:
		return jsonOutput(release.FileItems)
	case outputFormatTable:
		return tableOutput()
	default:
		return fmt.Errorf(
			`unable to match a printer suitable for the output format "%s", allowed formats are: json,content`,
			outputFormat)
	}
}

func runGetFileContents(bscp client.Client, app string, contentIDs []string) error {
	release, err := bscp.PullFiles(app, client.WithAppLabels(labels))
	if err != nil {
		return err
	}

	fileMap := make(map[string]*client.ConfigItemFile)
	allFiles := make([]*client.ConfigItemFile, len(release.FileItems))
	for idx, f := range release.FileItems {
		fileMap[f.FileMeta.ContentSpec.Signature] = f
		allFiles[idx] = f
	}

	files := allFiles
	if len(contentIDs) > 0 {
		files = []*client.ConfigItemFile{}
		for _, id := range contentIDs {
			if _, ok := fileMap[id]; !ok {
				return fmt.Errorf("the file content id %s is not exist for the latest release of app %s", id, app)
			}
			files = append(files, fileMap[id])
		}
	}

	var contents [][]byte
	contents, err = getfileContents(files)
	if err != nil {
		return err
	}

	// output only content when getting for just one file which is convenient to save it directly in a file
	if len(contentIDs) == 1 {
		_, err = fmt.Fprint(os.Stdout, string(contents[0]))
		return err
	}

	output := ""
	for idx, file := range files {
		output += fmt.Sprintf("***start No.%d***\nfile: %s\ncontentID: %s\nconent: \n%s\n***end No.%d***\n\n",
			idx+1, path.Join(file.Path, file.Name), file.FileMeta.ContentSpec.Signature, contents[idx], idx+1)
	}

	_, err = fmt.Fprint(os.Stdout, output)
	return err
}

// getfileContents get file contents concurrently
func getfileContents(files []*client.ConfigItemFile) ([][]byte, error) {
	contents := make([][]byte, len(files))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(10)

	for i, f := range files {
		idx, file := i, f
		g.Go(func() error {
			content, err := file.GetContent()
			if err != nil {
				return err
			}
			contents[idx] = content
			return nil
		})
	}

	return contents, g.Wait()
}

// runGetFile executes the get file command.
func runGetFile(args []string) error {
	baseConf, err := initBaseConf()
	if err != nil {
		return err
	}

	if appName == "" {
		return fmt.Errorf("app must not be empty")
	}

	bscp, err := client.New(
		client.WithFeedAddrs(baseConf.GetFeedAddrs()),
		client.WithBizID(baseConf.Biz),
		client.WithToken(baseConf.Token),
		client.WithFileCache(client.FileCache{
			Enabled:     *baseConf.FileCache.Enabled,
			CacheDir:    baseConf.FileCache.CacheDir,
			ThresholdGB: baseConf.FileCache.ThresholdGB,
		}),
	)

	if err != nil {
		return err
	}

	if outputFormat == outputFormatContent {
		return runGetFileContents(bscp, appName, args)
	}

	return runGetFileList(bscp, appName, args)
}

func runGetKvList(bscp client.Client, app string, match []string) error {
	release, err := bscp.PullKvs(app, match, client.WithAppLabels(labels))
	if err != nil {
		return err
	}

	tableOutput := func() error {
		table := newTable()
		table.SetHeader([]string{"Key", "Type", "Reviser", "UpdateAt"})

		for _, v := range release.KvItems {
			table.Append([]string{
				v.Key,
				v.KvType,
				v.Revision.Reviser,
				refineOutputTime(v.Revision.UpdateAt),
			})
		}

		table.Render()
		return nil
	}

	switch outputFormat {
	case outputFormatJson:
		return jsonOutput(release.KvItems)
	case outputFormatTable:
		return tableOutput()
	default:
		return fmt.Errorf(
			`unable to match a printer suitable for the output format "%s", allowed formats are: json,value,value_json`,
			outputFormat)
	}
}

func runGetKvValue(bscp client.Client, app, key string) error {
	value, err := bscp.Get(app, key, client.WithAppLabels(labels))
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(os.Stdout, value)
	return err
}

func runGetKvValues(bscp client.Client, app string, keys []string) error {
	release, err := bscp.PullKvs(app, []string{}, client.WithAppLabels(labels))
	if err != nil {
		return err
	}
	kvTypeMap := make(map[string]string)
	isAll := false
	if len(keys) == 0 {
		isAll = true
	}
	for _, k := range release.KvItems {
		kvTypeMap[k.Key] = k.KvType
		if isAll {
			keys = append(keys, k.Key)
		}
	}

	var values []string
	values, err = getKvValues(bscp, app, keys)
	if err != nil {
		return err
	}

	output := make(map[string]any, len(keys))
	for idx, key := range keys {
		output[key] = map[string]string{
			"kv_type": kvTypeMap[key],
			"value":   values[idx],
		}
	}

	return jsonOutput(output)
}

// getKvValues get kv values concurrently
func getKvValues(bscp client.Client, app string, keys []string) ([]string, error) {
	values := make([]string, len(keys))
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(10)

	for i, k := range keys {
		idx, key := i, k
		g.Go(func() error {
			value, err := bscp.Get(app, key, client.WithAppLabels(labels))
			if err != nil {
				return err
			}
			values[idx] = value
			return nil
		})
	}

	return values, g.Wait()
}

// runGetKv executes the get kv command.
func runGetKv(args []string) error {
	baseConf, err := initBaseConf()
	if err != nil {
		return err
	}

	if appName == "" {
		return fmt.Errorf("app must not be empty")
	}

	bscp, err := client.New(
		client.WithFeedAddrs(baseConf.GetFeedAddrs()),
		client.WithBizID(baseConf.Biz),
		client.WithToken(baseConf.Token),
	)

	if err != nil {
		return err
	}

	switch outputFormat {
	case outputFormatValue:
		if len(args) == 0 {
			return fmt.Errorf("res must not be empty")
		}
		if len(args) > 1 {
			return fmt.Errorf("multiple res are not supported")
		}
		return runGetKvValue(bscp, appName, args[0])
	case outputFormatValueJson:
		return runGetKvValues(bscp, appName, args)
	default:
		return runGetKvList(bscp, appName, args)
	}
}
