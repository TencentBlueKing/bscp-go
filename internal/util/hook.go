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
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/dal/table"
	pbhook "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/hook"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/pkg/env"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	// executeShellCmd shell script executor
	executeShellCmd = "bash"
	// executePythonCmd python script executor
	executePythonCmd = "python3"
)

// ExecuteHook executes the hook.
func ExecuteHook(hook *pbhook.HookSpec, hookType table.HookType,
	tempDir string, biz uint32, app string, relName string) error {
	appTempDir := path.Join(tempDir, fmt.Sprintf("%d/%s", biz, app))
	hookEnvs := []string{
		fmt.Sprintf("%s=%s", env.HookAppTempDir, appTempDir),
		fmt.Sprintf("%s=%s", env.HookTempDir, tempDir),
		fmt.Sprintf("%s=%d", env.HookBiz, biz),
		fmt.Sprintf("%s=%s", env.HookApp, app),
		fmt.Sprintf("%s=%s", env.HookRelName, relName),
	}

	hookPath, err := saveContentToFile(appTempDir, hook, hookType, hookEnvs)
	if err != nil {
		logger.Error("save hook content to file failed", logger.ErrAttr(err))
		return err
	}
	var command string
	switch hook.Type {
	case "shell":
		command = executeShellCmd
	case "python":
		command = executePythonCmd
	default:
		return sfs.WrapSecondaryError(sfs.ScriptTypeNotSupported, fmt.Errorf("invalid hook type: %s", hook.Type))
	}
	args := []string{hookPath}
	cmd := exec.Command(command, args...)
	cmd.Dir = appTempDir
	cmd.Env = append(os.Environ(), hookEnvs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return sfs.WrapSecondaryError(sfs.ScriptExecutionFailed,
			fmt.Errorf("exec %s error: %s, output: %s", hookType.String(), err.Error(), string(out)))
	}
	logger.Info("exec hook success", slog.String("script", hookType.String()), slog.String("output", string(out)))
	return nil
}

func saveContentToFile(workspace string, hook *pbhook.HookSpec, hookType table.HookType, hookEnvs []string) (string,
	error) {
	hookDir := path.Join(workspace, "hooks")
	if err := os.MkdirAll(hookDir, os.ModePerm); err != nil {
		logger.Error("mkdir hook dir failed", slog.String("dir", hookDir), logger.ErrAttr(err))
		return "", sfs.WrapSecondaryError(sfs.NewFolderFailed, err)
	}
	var filePath string
	switch hook.Type {
	case "shell":
		filePath = path.Join(hookDir, hookType.String()+".sh")
	case "python":
		filePath = path.Join(hookDir, hookType.String()+".py")
	default:
		return "", sfs.WrapSecondaryError(sfs.ScriptTypeNotSupported, fmt.Errorf("invalid hook type: %s", hook.Type))
	}
	if err := os.WriteFile(filePath, []byte(hook.Content), os.ModePerm); err != nil {
		logger.Error("write hook file failed", slog.String("file", filePath), logger.ErrAttr(err))
		return "", sfs.WrapSecondaryError(sfs.WriteFileFailed, err)
	}

	envfile := path.Join(hookDir, "env")
	if err := os.WriteFile(envfile, []byte("export "+strings.Join(hookEnvs, "\nexport ")+"\n"), 0644); err != nil {
		logger.Error("write hook env file failed", slog.String("file", envfile), logger.ErrAttr(err))
		return "", sfs.WrapSecondaryError(sfs.WriteEnvFileFailed, err)
	}

	return filePath, nil
}
