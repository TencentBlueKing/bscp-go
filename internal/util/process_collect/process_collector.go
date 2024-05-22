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

// Package process_collect xxx
package process_collect

import (
	"context"
	"os"
	"sync"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

type processCollector struct {
	pidFn func() (int, error)
	ctx   context.Context
}

var (
	once sync.Once
	// 当前cpu使用率
	cpuUsage float64
	// 最大cpu使用率
	cpuMaxUsage float64
	// 最小cpu使用率
	cpuMinUsage float64
	// 某段时间的cpu使用率
	cpuTotalUsage float64
	// 按秒累加计数
	cpuCount int
	// 当前内存使用量
	memoryUsage uint64
	// 最小内存使用量
	memoryMinUsage uint64
	// 某段时间的内存使用量
	memoryTotalUsage uint64
	// 按秒累加计数
	memoryCount int
	// 最大内存使用量
	memoryMaxUsage uint64
)

// NewProcessCollector xxx
func NewProcessCollector(ctx context.Context) {

	once.Do(func() {
		c := &processCollector{
			ctx: ctx,
		}
		c.pidFn = getPIDFn()
		// Set up process metric collection if supported by the runtime.
		if !canCollectProcess() {
			logger.Error("process metrics not supported on this platform")
			return
		}

		c.processCollect()
	})
}

// setCpuUsage set current, max, and min CPU usage
func setCpuUsage(usage float64) {
	cpuUsage = usage
	if cpuUsage > cpuMaxUsage {
		cpuMaxUsage = cpuUsage
	}
	if cpuUsage > 0 && cpuUsage < cpuMinUsage {
		cpuMinUsage = cpuUsage
	}
	cpuTotalUsage += cpuUsage
	cpuCount++
}

// setMemUsage set current, max, and min memory usage
func setMemUsage(usage uint64) {
	memoryUsage = usage
	if memoryUsage > memoryMaxUsage {
		memoryMaxUsage = memoryUsage
	}
	if memoryUsage > 0 && memoryUsage < memoryMinUsage {
		memoryMinUsage = memoryUsage
	}
	memoryTotalUsage += memoryUsage
	memoryCount++
}

// GetCpuUsage returns current, max, min, and average CPU usage
func GetCpuUsage() (usage, max, min, avg float64) {
	if cpuCount == 0 {
		avg = 0
	} else {
		avg = cpuTotalUsage / float64(cpuCount)
	}
	return cpuUsage, cpuMaxUsage, cpuMinUsage, avg
}

// GetMemUsage returns current, max, min, and average memory usage
func GetMemUsage() (usage, max, min, avg uint64) {
	if memoryCount == 0 {
		avg = 0
	} else {
		avg = memoryTotalUsage / uint64(memoryCount)
	}
	return memoryUsage, memoryMaxUsage, memoryMinUsage, avg
}

// get the current process
func getPIDFn() func() (int, error) {
	pid := os.Getpid()
	return func() (int, error) {
		return pid, nil
	}
}
