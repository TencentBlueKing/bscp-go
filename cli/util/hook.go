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

package util

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	"bscp.io/pkg/dal/table"
	"bscp.io/pkg/logs"
	pbhook "bscp.io/pkg/protocol/core/hook"
)

const (
	// executeShellCmd shell script executor
	executeShellCmd = "bash"
	// executePythonCmd python script executor
	executePythonCmd = "python3"
)

// ExecuteHook executes the hook.
func ExecuteHook(workspace string, hook *pbhook.HookSpec, hookType table.HookType) error {
	switch hook.Type {
	case "shell":
		return ExecuteShellHook(workspace, hook, hookType)
	case "python":
		return ExecutePythonHook(workspace, hook, hookType)
	default:
		return fmt.Errorf("invalid hook type: %s", hook.Type)
	}
}

// ExecuteShellHook executes the shell hook.
func ExecuteShellHook(workspace string, hook *pbhook.HookSpec, hookType table.HookType) error {
	hookPath, err := saveContentToFile(workspace, hook, hookType)
	if err != nil {
		logs.Errorf("save hook content to file error: %s", err.Error())
		return err
	}
	args := []string{hookPath}
	cmd := exec.Command(executeShellCmd, args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exec %s error: %s, output: %s", hookType.String(), err.Error(), string(out))
	}
	logs.Infof("exec %s success, output: \n%s", hookType.String(), string(out))
	return nil
}

// ExecutePythonHook executes the python hook.
func ExecutePythonHook(workspace string, hook *pbhook.HookSpec, hookType table.HookType) error {
	hookPath, err := saveContentToFile(workspace, hook, hookType)
	if err != nil {
		logs.Errorf("save hook content to file error: %s", err.Error())
		return err
	}
	args := []string{hookPath}
	cmd := exec.Command(executePythonCmd, args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exec %s error: %s, output: %s", hookType.String(), err.Error(), string(out))
	}
	logs.Infof("exec %s success, output: \n%s", hookType.String(), string(out))
	return nil
}

func saveContentToFile(workspace string, hook *pbhook.HookSpec, hookType table.HookType) (string, error) {
	hookDir := path.Join(workspace, "hooks")
	if err := os.MkdirAll(hookDir, os.ModePerm); err != nil {
		logs.Errorf("mkdir hook dir %s error: %+v", hookDir, err)
		return "", err
	}
	var filePath string
	switch hook.Type {
	case "shell":
		filePath = path.Join(hookDir, hookType.String()+".sh")
	case "python":
		filePath = path.Join(hookDir, hookType.String()+".py")
	default:
		return "", fmt.Errorf("invalid hook type: %s", hook.Type)
	}
	if err := os.WriteFile(filePath, []byte(hook.Content), os.ModePerm); err != nil {
		logs.Errorf("write hook file %s error: %s", filePath, err.Error())
		return "", err
	}
	return filePath, nil
}
