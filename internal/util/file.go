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
	"os"
	"os/user"
	"strconv"
	"strings"

	pbci "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/config-item"
)

// SetFilePermission sets the file permission.
func SetFilePermission(filePath string, pm *pbci.FilePermission) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open the target file failed, err: %v", err)
	}
	defer file.Close()

	mode, err := strconv.ParseInt("0"+pm.Privilege, 8, 64)
	if err != nil {
		return fmt.Errorf("parse %s privilege to int failed, err: %v", pm.Privilege, err)
	}

	if err = file.Chmod(os.FileMode(mode)); err != nil {
		return fmt.Errorf("file chmod %o failed, err: %v", mode, err)
	}

	ur, err := user.Lookup(pm.User)
	if err != nil {
		return fmt.Errorf("look up %s user failed, err: %v", pm.User, err)
	}

	uid, err := strconv.Atoi(ur.Uid)
	if err != nil {
		return fmt.Errorf("atoi %s uid failed, err: %v", ur.Uid, err)
	}

	gp, err := user.LookupGroup(pm.UserGroup)
	if err != nil {
		return fmt.Errorf("look up %s group failed, err: %v", pm.User, err)
	}

	gid, err := strconv.Atoi(gp.Gid)
	if err != nil {
		return fmt.Errorf("atoi %s gid failed, err: %v", gp.Gid, err)
	}

	if err := file.Chown(uid, gid); err != nil {
		return fmt.Errorf("file chown %s %s failed, err: %v", ur.Uid, gp.Gid, err)
	}

	return nil
}

// ConvertTextLineBreak converts the text file line break type.
func ConvertTextLineBreak(filePath string, lineBreak string) error {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// 将所有换行符规范化为 LF，使函数可重入执行
	normalizedContent := strings.ReplaceAll(string(content), "\r\n", "\n")
	normalizedContent = strings.ReplaceAll(normalizedContent, "\r", "\n")

	var targetLineBreak string
	switch lineBreak {
	case "", "LF":
		// default line break type is LF, no need to convert
		targetLineBreak = "\n"
	case "CRLF":
		targetLineBreak = "\r\n"
	case "CR":
		targetLineBreak = "\r"
	default:
		return fmt.Errorf("invalid line break type: %s", lineBreak)
	}

	// 替换换行符
	updatedContent := strings.ReplaceAll(normalizedContent, "\n", targetLineBreak)

	// 写回文件
	return os.WriteFile(filePath, []byte(updatedContent), 0644)
}
