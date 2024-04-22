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

package process_collect

import (
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

func canCollectProcess() bool {
	return true
}

var (
	modpsapi    = syscall.NewLazyDLL("psapi.dll")
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = modkernel32.NewProc("GetProcessHandleCount")
)

type processMemoryCounters struct {
	// System interface description
	// https://docs.microsoft.com/en-us/windows/desktop/api/psapi/ns-psapi-process_memory_counters_ex

	// Refer to the Golang internal implementation
	// https://golang.org/src/internal/syscall/windows/psapi_windows.go
	_                          uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

func getProcessMemoryInfo(handle windows.Handle) (processMemoryCounters, error) {
	mem := processMemoryCounters{}
	r1, _, err := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&mem)),
		uintptr(unsafe.Sizeof(mem)),
	)
	if r1 != 1 {
		return mem, err
	} else {
		return mem, nil
	}
}

func getProcessHandleCount(handle windows.Handle) (uint32, error) {
	var count uint32
	r1, _, err := procGetProcessHandleCount.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 != 1 {
		return 0, err
	} else {
		return count, nil
	}
}

func (c *processCollector) processCollect() {
	h := windows.CurrentProcess()
	var startTime, exitTime, kernelTime, userTime windows.Filetime

	for {
		select {
		case <-time.After(time.Second):
			if err := windows.GetProcessTimes(h, &startTime, &exitTime, &kernelTime, &userTime); err != nil {
				logger.Error("get process times", logger.ErrAttr(err))
				return
			}

			// set cpu usage
			setCpuUsage(fileTimeToSeconds(kernelTime) + fileTimeToSeconds(userTime))

			mem, err := getProcessMemoryInfo(h)
			if err != nil {
				logger.Error("get process memory info", logger.ErrAttr(err))
				return
			}
			// set memory usage
			setMemUsage(uint64(mem.WorkingSetSize))

			if _, err := getProcessHandleCount(h); err != nil {
				logger.Error("get process handle count", logger.ErrAttr(err))
				return
			}

		case <-c.ctx.Done():
			return
		}
	}
}

func fileTimeToSeconds(ft windows.Filetime) float64 {
	return float64(uint64(ft.HighDateTime)<<32+uint64(ft.LowDateTime)) / 1e7
}
