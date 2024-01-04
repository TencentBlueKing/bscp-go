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

package util_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/dal/table"
	pbhook "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/hook"

	"github.com/TencentBlueKing/bscp-go/cmd/bscp/internal/util"
)

func TestExecuteShellHook(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hook-test")
	if err != nil {
		t.Fatalf("create workspace error: %s", err.Error())
	}
	defer os.RemoveAll(tempDir)

	biz := 2
	app := "bscp-test"
	appTempDir := filepath.Join(tempDir, strconv.Itoa(biz), app)

	hookContent := "#!/bin/sh\nmkdir -p test"
	hookSpec := &pbhook.HookSpec{
		Name:    "test-shell",
		Type:    "shell",
		Content: hookContent,
	}

	err = util.ExecuteHook(hookSpec, table.PreHook, tempDir, 2, "bscp-test")
	if err != nil {
		t.Fatalf("execute hook error: %s", err.Error())
	}

	hookPath := filepath.Join(appTempDir, "hooks", table.PreHook.String()+".sh")
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Fatalf("hook file not exist: %s", hookPath)
	}

	if _, err := os.Stat(filepath.Join(appTempDir, "test")); os.IsNotExist(err) {
		t.Fatal("hook exec failed")
	}
}

func TestExecutePythonHook(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hook-test")
	if err != nil {
		t.Fatalf("create workspace error: %s", err.Error())
	}
	defer os.RemoveAll(tempDir)

	biz := 2
	app := "bscp-test"
	appTempDir := filepath.Join(tempDir, strconv.Itoa(biz), app)

	hookContent := "import os\nos.makedirs('test')"
	hookSpec := &pbhook.HookSpec{
		Name:    "test-python",
		Type:    "python",
		Content: hookContent,
	}

	err = util.ExecuteHook(hookSpec, table.PostHook, tempDir, 2, "bscp-test")
	if err != nil {
		t.Fatalf("execute hook error: %s", err.Error())
	}

	hookPath := filepath.Join(appTempDir, "hooks", table.PostHook.String()+".py")
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Fatalf("hook file not exist: %s", hookPath)
	}

	if _, err := os.Stat(filepath.Join(appTempDir, "test")); os.IsNotExist(err) {
		t.Fatal("hook exec failed")
	}
}

func TestExecuteShellHookFailed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hook-test")
	if err != nil {
		t.Fatalf("create workspace error: %s", err.Error())
	}
	defer os.RemoveAll(tempDir)
	hookContent := "#!/bin/sh\nmkdir -p test\ncmd-not-exists"
	hookSpec := &pbhook.HookSpec{
		Name:    "test-shell",
		Type:    "shell",
		Content: hookContent,
	}

	err = util.ExecuteHook(hookSpec, table.PreHook, tempDir, 2, "bscp-test")
	if err == nil {
		t.Fatalf("execute hook unexpected, should failed but not.")
	}
}

func TestExecutePythonHookFailed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hook-test")
	if err != nil {
		t.Fatalf("create workspace error: %s", err.Error())
	}
	defer os.RemoveAll(tempDir)

	hookContent := "import os\nos.makedirs('test')\n1/0"
	hookSpec := &pbhook.HookSpec{
		Name:    "test-python",
		Type:    "python",
		Content: hookContent,
	}

	err = util.ExecuteHook(hookSpec, table.PostHook, tempDir, 2, "bscp-test")
	if err == nil {
		t.Fatalf("execute hook unexpected, should failed but not.")
	}
}
