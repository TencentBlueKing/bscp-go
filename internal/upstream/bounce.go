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

package upstream

import (
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/tools"
	"go.uber.org/atomic"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const defaultBounceIntervalHour = 1

// bounce define connect bounce manager.
type bounce struct {
	reconnectFunc func() error
	intervalHour  *atomic.Uint32
	st            *atomic.Bool
}

func initBounce(reconnectFunc func() error) *bounce {
	bc := &bounce{
		intervalHour:  atomic.NewUint32(defaultBounceIntervalHour),
		reconnectFunc: reconnectFunc,
		st:            atomic.NewBool(false),
	}

	return bc
}

func (b *bounce) state() bool {
	return b.st.Load()
}

func (b *bounce) updateInterval(intervalHour uint) {
	b.intervalHour.Store(uint32(intervalHour))
}

// enableBounce wait for the bounce to be reached and to reconnect upstream server.
// with each call, reschedule bounce time.
func (b *bounce) enableBounce() {
	if b.st.Load() {
		logger.Error("bounce is enabled state, unable to enable bounce again")
		return
	}

	b.st.Store(true)

	for {
		intervalHour := b.intervalHour.Load()

		logger.Info("start wait connect bounce, bounce interval", slog.Any("intervalHour", intervalHour))

		time.Sleep(time.Duration(intervalHour) * time.Hour)

		logger.Info("reach the bounce time and start to reconnect stream server")

		retry := tools.NewRetryPolicy(5, [2]uint{500, 15000})
		for {
			if err := b.reconnectFunc(); err != nil {
				logger.Error("reconnect upstream server failed", logger.ErrAttr(err))
				retry.Sleep()
				continue
			}

			logger.Info("reconnect new upstream server success.")
			break
		}
	}
}
