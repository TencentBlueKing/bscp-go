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

package client

import (
	"strconv"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/tools"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

// NotifyReconnect notify the watcher to reconnect the upstream server.
func (w *watcher) NotifyReconnect(signal reconnectSignal) {
	select {
	case w.reconnectChan <- signal:
	default:
		logger.Info("reconnect signal channel size is full, skip this signal", slog.String("reason", signal.Reason))
	}
}

func (w *watcher) waitForReconnectSignal() {
	for {
		select {
		case <-w.vas.Ctx.Done():
			return
		case signal := <-w.reconnectChan:
			logger.Info("received reconnect signal", slog.String("reason", signal.String()), slog.String("rid", w.vas.Rid))

			// stop the previous watch stream before close conn.
			w.StopWatch()
			w.tryReconnect(w.vas.Rid)
			return
		}
	}
}

// tryReconnect, Use NotifyReconnect method instead of direct call
func (w *watcher) tryReconnect(rid string) {
	st := time.Now()
	logger.Info("start to reconnect the upstream server", slog.String("rid", w.vas.Rid))

	retry := tools.NewRetryPolicy(5, [2]uint{500, 15000})
	for {
		subRid := rid + strconv.FormatUint(uint64(retry.RetryCount()), 10)

		if err := w.upstream.ReconnectUpstreamServer(); err != nil {
			logger.Error("reconnect upstream server failed", logger.ErrAttr(err), slog.String("rid", subRid))
			retry.Sleep()
			continue
		}

		logger.Info("reconnect new upstream server success", slog.String("rid", subRid))
		break
	}

	for {
		subRid := rid + strconv.FormatUint(uint64(retry.RetryCount()), 10)
		if e := w.StartWatch(); e != nil {
			logger.Error("re-watch stream failed", logger.ErrAttr(e), slog.String("rid", subRid))
			retry.Sleep()
			continue
		}

		logger.Info("re-watch stream success", slog.String("rid", subRid))
		break
	}

	logger.Info("reconnect and re-watch the upstream server done",
		slog.String("rid", rid), slog.Duration("duration", time.Since(st)))
}
