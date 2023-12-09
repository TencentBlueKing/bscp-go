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

// pull kv example for bscp sdk
package main

import (
	"os"
	"strconv"
	"strings"

	"bscp.io/pkg/logs"

	"github.com/TencentBlueKing/bscp-go/cli/config"
	"github.com/TencentBlueKing/bscp-go/client"
	"github.com/TencentBlueKing/bscp-go/option"
)

func main() {
	logs.InitLogger(logs.LogConfig{ToStdErr: true, LogLineMaxSize: 1000})

	// 初始化配置信息, 按需修改
	bizStr := os.Getenv("BSCP_BIZ")
	biz, err := strconv.ParseInt(bizStr, 10, 64)
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}

	conf := &config.ClientConfig{
		FeedAddrs: strings.Split(os.Getenv("BSCP_FEED_ADDRS"), ","),
		Biz:       uint32(biz),
		Token:     os.Getenv("BSCP_TOKEN"),
	}

	bscp, err := client.New(
		option.FeedAddrs(conf.FeedAddrs),
		option.BizID(conf.Biz),
		option.Token(conf.Token),
	)
	if err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}

	appName := os.Getenv("BSCP_APP")
	opts := []option.AppOption{}
	key := "key1"
	if err = pullAppKvs(bscp, appName, key, opts); err != nil {
		logs.Errorf(err.Error())
		os.Exit(1)
	}
}

// pullAppKvs 拉取 key 的值
func pullAppKvs(bscp client.Client, app string, key string, opts []option.AppOption) error {
	value, err := bscp.Get(app, key, opts...)
	if err != nil {
		return err
	}

	logs.Infof("get %s value %s", key, value)
	return nil
}