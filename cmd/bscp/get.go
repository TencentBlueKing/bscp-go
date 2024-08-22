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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/internal/constant"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	outputFormat string
	downloadDir  string
	ignoreDir    bool
)

const (
	outputFormatTable     = ""
	outputFormatJson      = "json"
	outputFormatValue     = "value"
	outputFormatValueJson = "value_json"
	outputFormatContent   = "content"
)

var (
	// getCmd is parent cmd for other sub cmds
	getCmd = &cobra.Command{
		Use:   "get",
		Short: "Display app, file or kv resources",
		Long:  `Display app, file or kv resources`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 设置日志等级, get 命令默认是 error
			logLevel := getViper.GetString("log_level")
			if logLevel == "" {
				logLevel = "error"
			}

			level := logger.GetLevelByName(logLevel)
			logger.SetLevel(level)

			return nil
		},
	}

	// getAppCmd is get cmd for app
	getAppCmd = &cobra.Command{
		Use:   "app [res...]",
		Short: "Display app resources",
		Long:  `Display app resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetApp(args)
		},
	}

	// getFileCmd is get cmd for file
	getFileCmd = &cobra.Command{
		Use:   "file [res...]",
		Short: "Display file resources",
		Long:  `Display file resources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetFile(args)
		},
	}

	// getKvCmd is get cmd for kv
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
	getCmd.PersistentFlags().StringP("feed-addrs", "f", "", "feed server address, eg: 'bscp-feed.example.com:9510'")
	getCmd.PersistentFlags().IntP("biz", "b", 0, "biz id")
	getCmd.PersistentFlags().StringP("token", "t", "", "sdk token")
	for _, v := range getVipers {
		mustBindPFlag(v, "feed_addrs", getCmd.PersistentFlags().Lookup("feed-addrs"))
		mustBindPFlag(v, "biz", getCmd.PersistentFlags().Lookup("biz"))
		mustBindPFlag(v, "token", getCmd.PersistentFlags().Lookup("token"))

		for key, envName := range commonEnvs {
			// bind env variable with viper
			if err := v.BindEnv(key, envName); err != nil {
				panic(err)
			}
		}
	}

	// app 参数
	getAppCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "output format, One of: json")

	// file 参数
	getFileCmd.Flags().StringP("app", "a", "", "app name")
	mustBindPFlag(getFileViper, "app", getFileCmd.Flags().Lookup("app"))
	getFileCmd.Flags().StringP("labels", "l", "", "labels")
	mustBindPFlag(getFileViper, "labels_str", getFileCmd.Flags().Lookup("labels"))
	getFileCmd.Flags().BoolP("file-cache-enabled", "", constant.DefaultFileCacheEnabled, "enable file cache or not")
	mustBindPFlag(getFileViper, "file_cache.enabled", getFileCmd.Flags().Lookup("file-cache-enabled"))
	getFileCmd.Flags().StringP("file-cache-dir", "", constant.DefaultFileCacheDir, "bscp file cache dir")
	mustBindPFlag(getFileViper, "file_cache.cache_dir", getFileCmd.Flags().Lookup("file-cache-dir"))
	getFileCmd.Flags().Float64P("cache-threshold-gb", "", constant.DefaultCacheThresholdGB,
		"bscp file cache threshold gigabyte")
	mustBindPFlag(getFileViper, "file_cache.threshold_gb", getFileCmd.Flags().Lookup("cache-threshold-gb"))
	getFileCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "output format, One of: json|content")
	getFileCmd.Flags().StringVarP(&downloadDir, "download-dir", "d", "",
		"the directory for saving the downloaded content")
	getFileCmd.Flags().BoolVar(&ignoreDir, "ignore-dir", false,
		"ignore directory hierarchy when downloading files, must be used with -d option")

	// kv 参数
	getKvCmd.Flags().StringP("app", "a", "", "app name")
	mustBindPFlag(getKvViper, "app", getKvCmd.Flags().Lookup("app"))
	getKvCmd.Flags().StringP("labels", "l", "", "labels")
	mustBindPFlag(getKvViper, "labels_str", getKvCmd.Flags().Lookup("labels"))
	getKvCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "output format, One of: json|value|value_json")

	for key, envName := range commonEnvs {
		// add env info for cmdline flags
		if f := getCmd.PersistentFlags().Lookup(strings.ReplaceAll(key, "_", "-")); f != nil {
			f.Usage = fmt.Sprintf("%v [env %v]", f.Usage, envName)
		}
		if f := getFileCmd.Flags().Lookup(strings.ReplaceAll(key, "_", "-")); f != nil {
			f.Usage = fmt.Sprintf("%v [env %v]", f.Usage, envName)
		}
		if f := getKvCmd.Flags().Lookup(strings.ReplaceAll(key, "_", "-")); f != nil {
			f.Usage = fmt.Sprintf("%v [env %v]", f.Usage, envName)
		}
	}
}

// runGetApp executes the get app command.
func runGetApp(args []string) error {
	if err := initConf(getAppViper); err != nil {
		logger.Error("init conf failed", logger.ErrAttr(err))
		os.Exit(1)
	}
	if err := conf.ValidateBase(); err != nil {
		logger.Error("validate base config failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
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

// runGetFileList gets file list
func runGetFileList(bscp client.Client, app string, match []string) error {
	release, err := getFileRelease(bscp, app, match)
	if err != nil {
		return err
	}

	tableOutput := func() error {
		table := newTable()
		table.SetHeader([]string{"File", "SHA256", "MD5", "Size", "Reviser", "UpdateAt"})
		for _, v := range release.FileItems {
			table.Append([]string{
				filepath.Join(v.Path, v.Name),
				v.FileMeta.ContentSpec.Signature,
				v.FileMeta.ContentSpec.Md5,
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

// runGetFileContents gets file contents
func runGetFileContents(bscp client.Client, app string, match []string) error {
	output, err := getFileOutput(bscp, app, match)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, output)
	return err
}

// getFileRelease gets file release
func getFileRelease(bscp client.Client, app string, match []string) (*client.Release, error) {
	var opts []client.AppOption
	if len(match) > 0 {
		opts = append(opts, client.WithAppConfigMatch(match))
	}
	opts = append(opts, client.WithAppLabels(conf.Labels))
	return bscp.PullFiles(app, opts...)
}

// getFileOutput gets file output
func getFileOutput(bscp client.Client, app string, match []string) (string, error) {
	release, err := getFileRelease(bscp, app, match)
	if err != nil {
		return "", err
	}

	var contents [][]byte
	contents, err = getfileContents(release.FileItems)
	if err != nil {
		return "", err
	}

	// output only content when getting for just one file which is convenient to save it directly in a file
	if len(release.FileItems) == 1 {
		return string(contents[0]), nil
	}

	output := ""
	for idx, file := range release.FileItems {
		output += fmt.Sprintf("***start No.%d***\nfile: %s\nconent: \n%s\n***end No.%d***\n\n",
			idx+1, filepath.Join(file.Path, file.Name), contents[idx], idx+1)
	}
	return output, nil
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

// runDownloadFile downloads file
func runDownloadFile(bscp client.Client, app string, match []string) error {
	// check if download directory exists
	fileInfo, err := os.Stat(downloadDir)
	if err != nil {
		return fmt.Errorf("check download directory %s failed, err: %s", downloadDir, err)
	}
	if !fileInfo.IsDir() {
		return fmt.Errorf("check download directory %s failed, err: path exists but is not a directory", downloadDir)
	}

	release, err := getFileRelease(bscp, app, match)
	if err != nil {
		return err
	}
	if len(release.FileItems) == 0 {
		fmt.Println("no matched files to download")
		return nil
	}

	dstFiles := make([]string, len(release.FileItems))
	var dstFile string
	fileNames := make(map[string][]string)
	for idx, f := range release.FileItems {
		if ignoreDir {
			dstFile = filepath.Join(downloadDir, f.Name)
			// used to check files with the same name when --ignore-dir is enabled
			fileNames[f.Name] = append(fileNames[f.Name], filepath.Join(f.Path, f.Name))
		} else {
			dstFile = filepath.Join(downloadDir, f.Path, f.Name)
		}
		dstFiles[idx] = dstFile
	}
	var sameFiles [][]string
	for _, names := range fileNames {
		if len(names) >= 2 {
			sameFiles = append(sameFiles, names)
		}
	}
	if len(sameFiles) > 0 {
		return fmt.Errorf("the file names are same for files in %v, we don't download and override directly, "+
			"you can remove the arg --ignore-dir or make your other choices", sameFiles)
	}

	// save content to dst file
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(10)
	for i, f := range release.FileItems {
		idx, file := i, f
		g.Go(func() error {
			fileDir := filepath.Dir(dstFiles[idx])
			err := os.MkdirAll(fileDir, os.ModePerm)
			if err != nil {
				return err
			}
			fmt.Printf("saving to file %s\n", dstFiles[idx])
			return file.SaveToFile(dstFiles[idx])
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	fmt.Printf("saved %d files successfully\n", len(dstFiles))
	return nil
}

// runGetFile executes the get file command.
func runGetFile(args []string) error {
	if err := initConf(getFileViper); err != nil {
		logger.Error("init conf failed", logger.ErrAttr(err))
		os.Exit(1)
	}
	if err := conf.ValidateBase(); err != nil {
		logger.Error("validate base config failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	if conf.App == "" {
		return fmt.Errorf("app must not be empty")
	}

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
		client.WithFileCache(client.FileCache{
			Enabled:     conf.FileCache.Enabled,
			CacheDir:    conf.FileCache.CacheDir,
			ThresholdGB: conf.FileCache.ThresholdGB,
		}),
	)

	if err != nil {
		return err
	}

	if downloadDir != "" {
		return runDownloadFile(bscp, conf.App, args)
	}

	if outputFormat == outputFormatContent {
		return runGetFileContents(bscp, conf.App, args)
	}

	return runGetFileList(bscp, conf.App, args)
}

// runGetKvList gets kv list
func runGetKvList(bscp client.Client, app string, match []string) error {
	release, err := bscp.PullKvs(app, match, client.WithAppLabels(conf.Labels))
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

// runGetKvValue gets single kv value
func runGetKvValue(bscp client.Client, app, key string) error {
	value, err := bscp.Get(app, key, client.WithAppLabels(conf.Labels))
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(os.Stdout, value)
	return err
}

// runGetKvValues gets kv values
func runGetKvValues(bscp client.Client, app string, keys []string) error {
	release, err := bscp.PullKvs(app, []string{}, client.WithAppLabels(conf.Labels))
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
			value, err := bscp.Get(app, key, client.WithAppLabels(conf.Labels))
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
	if err := initConf(getKvViper); err != nil {
		logger.Error("init conf failed", logger.ErrAttr(err))
		os.Exit(1)
	}
	if err := conf.ValidateBase(); err != nil {
		logger.Error("validate base config failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	if conf.App == "" {
		return fmt.Errorf("app must not be empty")
	}

	bscp, err := client.New(
		client.WithFeedAddrs(conf.FeedAddrs),
		client.WithBizID(conf.Biz),
		client.WithToken(conf.Token),
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
		return runGetKvValue(bscp, conf.App, args[0])
	case outputFormatValueJson:
		return runGetKvValues(bscp, conf.App, args)
	default:
		return runGetKvList(bscp, conf.App, args)
	}
}
