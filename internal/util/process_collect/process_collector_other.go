// nolint
//go:build !windows && !js && !wasip1

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
	"time"

	"github.com/prometheus/procfs"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

func canCollectProcess() bool {
	_, err := procfs.NewDefaultFS()
	return err == nil
}

func (c *processCollector) processCollect() {
	pid, err := c.pidFn()
	if err != nil {
		logger.Error("get current process", logger.ErrAttr(err))
		return
	}

	p, err := procfs.NewProc(pid)
	if err != nil {
		logger.Error("greturns a process for the given pid under /proc", logger.ErrAttr(err))
		return
	}

	for {
		select {
		case <-time.After(time.Second):
			stat, err := p.Stat()
			if err != nil {
				logger.Error("returns the current status information of the process", logger.ErrAttr(err))
				return
			}
			// set cpu usage
			setCpuUsage(stat.CPUTime())
			// set memory usage
			setMemUsage(uint64(stat.ResidentMemory()))
		case <-c.ctx.Done():
			return
		}
	}

}
