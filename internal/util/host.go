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

package util

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"os"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

// getMACAddress 获取MAC地址
func getMACAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iFace := range interfaces {
		if len(iFace.HardwareAddr) > 0 {
			return iFace.HardwareAddr.String(), nil
		}
	}

	return "", fmt.Errorf("MAC address not found")
}

// GetClientIP 获取客户端IP
func GetClientIP() string {
	var (
		adders  []net.Addr
		addr    net.Addr
		ipNet   *net.IPNet
		isIpNet bool
		err     error
	)
	// 获取所有网卡
	if adders, err = net.InterfaceAddrs(); err != nil {
		logger.Error("net.Interfaces failed", logger.ErrAttr(err))
		return ""
	}
	// 取第一个非lo的网卡IP
	for _, addr = range adders {
		// 这个网络地址是IP地址: ipv4, ipv6
		if ipNet, isIpNet = addr.(*net.IPNet); isIpNet && !ipNet.IP.IsLoopback() {
			// 跳过IPV6
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

// GenerateClientID 生成客户端唯一凭证
func GenerateClientID() (string, error) {
	// 获取MAC地址
	macAddr, err := getMACAddress()
	if err != nil {
		return "", err
	}

	// 获取主机名
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	// 获取客户端IP地址
	ipAddr := GetClientIP()

	// 组合信息
	data := fmt.Sprintf("%s-%s-%s", macAddr, hostname, ipAddr)

	// 使用MD5哈希函数生成唯一标识符
	hashes := md5.New() // nolint
	hashes.Write([]byte(data))
	hash := hashes.Sum(nil)

	// 将哈希值转换为十六进制字符串
	return hex.EncodeToString(hash), nil
}
