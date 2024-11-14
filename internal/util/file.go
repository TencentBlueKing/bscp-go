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
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	pbci "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/config-item"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
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
		return fmt.Errorf("look up %s group failed, err: %v", pm.UserGroup, err)
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

// Permission 权限组结构
type Permission struct {
	User      string
	UserGroup string
	Uid       string
	Gid       string
}

// SetUserAndUserGroup 设置用户和用户组
func SetUserAndUserGroup(permission *pbci.FilePermission) error {

	p := Permission{
		User:      permission.User,
		Uid:       fmt.Sprintf("%d", permission.Uid),
		UserGroup: permission.UserGroup,
		Gid:       fmt.Sprintf("%d", permission.Gid),
	}

	// 设置用户组
	if err := p.setUserGroup(); err != nil {
		return err
	}

	// 设置用户
	if err := p.setUser(); err != nil {
		return err
	}

	return addUserToGroup(p.User, p.UserGroup)
}

// SetUser 设置用户
func (p Permission) setUser() error {
	if checkUserAndGroup(p.User, p.Uid) {
		return nil
	}

	// 2. 获取用户信息
	usr, err := user.Lookup(p.User)
	if err == nil && usr.Uid == p.Uid {
		return nil
	}

	// 删除用户
	if err := deleteUser(p.User); err != nil {
		return err
	}

	// 通过UID删除用户
	if err := deleteUserByUID(p.Uid); err != nil {
		return err
	}

	// 创建用户
	if err := createUser(p.User, p.Uid, p.Gid); err != nil {
		return err
	}

	return nil
}

// addUserToGroup 将用户添加到某个用户组中
func addUserToGroup(username, groupname string) error {
	cmd := exec.Command("usermod", "-aG", groupname, username)
	return cmd.Run()
}

// deleteUser 删除用户
func deleteUser(username string) error {
	if checkUserAndGroup(username, "") {
		return nil
	}
	usr, err := user.Lookup(username)
	if err == nil {
		cmd := exec.Command("userdel", "-f", username)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s user deleted fialed, %s, err: %v", username, output, err)
		}
		logger.Info("user deleted success", slog.Any("user", username), slog.Any("uid", usr.Uid))
	}

	return nil
}

// deleteUserByUID 通过uid删除用户
func deleteUserByUID(uid string) error {
	usr, err := user.LookupId(uid)
	if err == nil {
		return deleteUser(usr.Username)
	}

	return nil
}

// createUser 创建用户
func createUser(username, uid, gid string) error {
	cmd := exec.Command("useradd", "-m", "-u", uid, "-g", gid, username)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s user created fialed, %s, err: %v", username, output, err)
	}
	logger.Info("user created success", slog.Any("user", username), slog.Any("uid", uid))

	return nil
}

// SetUserGroup 设置用户组
func (p Permission) setUserGroup() error {
	if checkUserAndGroup(p.UserGroup, p.Gid) {
		return nil
	}

	// 1. 通过组获取组的信息
	usr, err := user.LookupGroup(p.UserGroup)
	if err == nil && usr.Gid == p.Gid {
		return nil
	}

	// 删除用户组
	if err = deleteUserGroup(p.UserGroup); err != nil {
		return err
	}
	// 通过GID删除用户组
	if err = deleteUserGroupByGID(p.Gid); err != nil {
		return err
	}
	// 创建用户组
	if err = createUserGroup(p.UserGroup, p.Gid); err != nil {
		return err
	}

	return nil
}

// deleteUserGroup 删除用户组
func deleteUserGroup(groupname string) error {
	if checkUserAndGroup(groupname, "") {
		return nil
	}
	usr, err := user.LookupGroup(groupname)
	if err == nil {
		cmd := exec.Command("groupdel", "-f", groupname)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s group deleted failed, %s, err: %v", groupname, output, err)
		}
		logger.Info("group deleted success", slog.Any("userGroup", groupname), slog.Any("gid", usr.Gid))
	}

	return nil
}

// deleteUserGroupByGID 通过gid删除用户组
func deleteUserGroupByGID(gid string) error {
	usr, err := user.LookupGroupId(gid)
	if err == nil {
		return deleteUserGroup(usr.Name)
	}

	return nil
}

// createUserGroup 创建用户组
func createUserGroup(groupname, gid string) error {
	cmd := exec.Command("groupadd", "-g", gid, groupname)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s group created failed, %s, err: %v", groupname, output, err)
	}
	logger.Info("group created success", slog.Any("userGroup", groupname), slog.Any("gid", gid))

	return nil
}

// 检测用户和用户组
func checkUserAndGroup(name string, id string) bool {
	// 如果是root用户或用户组直接不处理
	if (id == "0" || id == "") && name == "root" {
		return true
	}

	return false
}
