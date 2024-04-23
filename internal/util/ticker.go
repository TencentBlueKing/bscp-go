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
	"sync"
	"time"
)

// ProgressiveTicker 结构体模拟 time.Ticker，但以递进式变化间隔
type ProgressiveTicker struct {
	C         chan time.Time // 用来接收 tick 事件
	durations []time.Duration
	stop      chan struct{}
	once      sync.Once // 用于确保只执行一次 Stop
}

// NewProgressiveTicker 创建并返回一个新的渐进式 ticker
// 默认按照 1s, 1s, 2s, 2s, 5s, 5s, 10s 的间隔递进，最终稳定在 10s
func NewProgressiveTicker(durations []time.Duration) *ProgressiveTicker {

	if durations == nil {
		durations = []time.Duration{
			time.Second,
			time.Second,
			2 * time.Second,
			2 * time.Second,
			5 * time.Second,
			5 * time.Second,
			10 * time.Second,
		}
	}

	tickChan := make(chan time.Time)
	stopChan := make(chan struct{})

	pt := &ProgressiveTicker{
		C:         tickChan,
		durations: durations,
		stop:      stopChan,
		once:      sync.Once{},
	}
	// start the ticker in a goroutine
	go pt.run()
	return pt
}

// Stop 停止 ticker，释放相关资源
func (pt *ProgressiveTicker) Stop() {
	pt.once.Do(func() {
		close(pt.stop)
	})
}

// run 是执行定时任务的协程
func (pt *ProgressiveTicker) run() {
	var fixedDuration time.Duration

	defer close(pt.C) // 确保关闭 channel
	for _, duration := range pt.durations {
		fixedDuration = duration
		timer := time.NewTimer(duration)
		select {
		case <-timer.C:
			pt.C <- time.Now()
			timer.Stop()
		case <-pt.stop:
			timer.Stop()
			return
		}
	}

	// 进入固定的无限循环，每固定时间发送一个 tick，直到 stop 被关闭
	fixedTicker := time.NewTicker(fixedDuration)
	for {
		select {
		case <-fixedTicker.C:
			pt.C <- time.Now()
		case <-pt.stop:
			fixedTicker.Stop()
			return
		}
	}
}
