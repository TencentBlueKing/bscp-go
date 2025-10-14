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

// download file bench test
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

func main() {

	// 设置日志自定义 Handler
	// logger.SetHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	// 在线服务, 可设置 metrics
	// metrics.RegisterMetrics()
	// http.Handle("/metrics", promhttp.Handler())

	var feedAddr = ""
	var biz = 0
	var token = ""
	var app = ""
	var targetFileName = ""
	labels := map[string]string{}

	start := time.Now()

	err := func() error {
		clientOpts := []client.Option{
			client.WithFeedAddr(feedAddr),
			client.WithBizID(uint32(biz)),
			client.WithToken(token),
			client.WithLabels(labels),
		}
		// 初始化客户端
		bscp, err := client.New(clientOpts...)
		if err != nil {
			return fmt.Errorf("init client failed, err: %v", err)
		}
		defer bscp.Close()
		opts := []client.AppOption{}
		// 拉取单文件
		reader, err := bscp.GetFile(app, targetFileName, opts...)
		if err != nil {
			return fmt.Errorf("get app file failed, err: %v", err)
		}
		defer func() {
			_ = reader.Close()
		}()

		// 设置文件保存路径
		savePath := ""
		fullPath := filepath.Join(savePath, targetFileName)

		// 创建输出文件
		outFile, err := os.Create(fullPath)
		if err != nil {
			return fmt.Errorf("create file failed, err: %v", err)
		}
		defer func() {
			_ = outFile.Close()
		}()

		// 将数据从 reader 复制到 outFile
		_, err = io.Copy(outFile, reader)
		if err != nil {
			return fmt.Errorf("copy data failed, err: %v", err)
		}

		logger.Debug("get file content by downloading from repo success",
			slog.String("file", fullPath))

		return nil
	}()

	costTime := time.Since(start).Seconds()
	// 判断是否出错，决定是否退出
	if err != nil {
		logger.Error("download app files failed", logger.ErrAttr(err))
		os.Exit(1)
	}

	logger.Info("download app files finished", slog.Float64("cost_time_seconds", costTime))
}
