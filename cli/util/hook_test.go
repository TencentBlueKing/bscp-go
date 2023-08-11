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

package util_test

import (
	"os"
	"path/filepath"
	"testing"

	"bscp.io/pkg/dal/table"
	pbhook "bscp.io/pkg/protocol/core/hook"
	"github.com/TencentBlueKing/bscp-go/cli/util"
)

func TestExecuteShellHook(t *testing.T) {
	workspace, err := os.MkdirTemp("", "hook-test")
	if err != nil {
		t.Fatalf("create workspace error: %s", err.Error())
	}
	defer os.RemoveAll(workspace)

	hookContent := "#!/bin/sh\nmkdir -p test"
	hookSpec := &pbhook.HookSpec{
		Name:    "test-shell",
		Type:    "shell",
		Content: hookContent,
	}

	err = util.ExecuteHook(workspace, hookSpec, table.PreHook)
	if err != nil {
		t.Fatalf("execute hook error: %s", err.Error())
	}

	hookPath := filepath.Join(workspace, "hooks", table.PreHook.String()+".sh")
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Fatalf("hook file not exist: %s", hookPath)
	}

	if _, err := os.Stat(filepath.Join(workspace, "test")); os.IsNotExist(err) {
		t.Fatal("hook exec failed")
	}
}

func TestExecutePythonHook(t *testing.T) {
	workspace, err := os.MkdirTemp("", "hook-test")
	if err != nil {
		t.Fatalf("create workspace error: %s", err.Error())
	}
	defer os.RemoveAll(workspace)

	hookContent := "import os\nos.makedirs('test')"
	hookSpec := &pbhook.HookSpec{
		Name:    "test-python",
		Type:    "python",
		Content: hookContent,
	}

	err = util.ExecuteHook(workspace, hookSpec, table.PostHook)
	if err != nil {
		t.Fatalf("execute hook error: %s", err.Error())
	}

	hookPath := filepath.Join(workspace, "hooks", table.PostHook.String()+".py")
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Fatalf("hook file not exist: %s", hookPath)
	}

	if _, err := os.Stat(filepath.Join(workspace, "test")); os.IsNotExist(err) {
		t.Fatal("hook exec failed")
	}
}
