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

// Package util defines the common util function.
package util

import (
	"fmt"
	"time"

	"github.com/denisbrodbeck/machineid"
)

// GenerateFingerPrint generate finger print
func GenerateFingerPrint() (string, error) {
	// 1. try to get machine id
	// 2. 在 WSL 系统下获取 machineID 不会报错但是拿到的是空字符串
	machineID, err := machineid.ID()
	if err == nil && machineID != "" {
		return machineID, nil
	}
	// 2. try to get id from
	fp, err := GenerateClientID()
	if err == nil {
		return fp, nil
	}
	return "", fmt.Errorf("failed to generate fingerprint, err %s", err.Error())
}

// GenerateCursorID 生成cursorID
func GenerateCursorID(bizID uint32) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%d%d", bizID, timestamp)
}
