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

package watch

import (
	"strconv"

	"bscp.io/pkg/tools"

	"github.com/TencentBlueKing/bscp-go/logger"
	"github.com/TencentBlueKing/bscp-go/types"
)

// NotifyReconnect notify the watcher to reconnect the upstream server.
func (w *Watcher) NotifyReconnect(signal types.ReconnectSignal) {
	select {
	case w.reconnectChan <- signal:
	default:
		logger.Infof("reconnect signal channel size is full, skip this signal, reason: %s", signal.Reason)
	}
}

func (w *Watcher) waitForReconnectSignal() {
	w.vas.Wg.Add(1)
	for {
		select {
		case <-w.vas.Ctx.Done():
			logger.Infof("wait for reconnect signal stoped, rid: %s", w.vas.Rid)
			w.vas.Wg.Done()
			return
		case signal := <-w.reconnectChan:
			logger.Infof("received reconnect signal, reason: %s, rid: %s", signal.String(), w.vas.Rid)

			if w.reconnecting.Load() {
				logger.Warnf("received reconnect signal, but stream is already reconnecting, ignore this signal.")
				return
			}

			w.tryReconnect(w.vas.Rid)
		}
	}
}

func (w *Watcher) tryReconnect(rid string) {
	w.reconnecting.Store(true)

	logger.Infof("start to reconnect the upstream server, rid: %s", rid)

	// stop the previous watch stream before close conn.
	w.StopWatch()

	retry := tools.NewRetryPolicy(5, [2]uint{500, 15000})
	for {
		subRid := rid + strconv.FormatUint(uint64(retry.RetryCount()), 10)

		if err := w.upstream.ReconnectUpstreamServer(); err != nil {
			logger.Errorf("reconnect upstream server failed, err: %s, rid: %s", err.Error(), subRid)
			retry.Sleep()
			continue
		}

		logger.Infof("reconnect new upstream server success. rid: %s", subRid)
		break
	}

	for {
		subRid := rid + strconv.FormatUint(uint64(retry.RetryCount()), 10)
		if e := w.StartWatch(); e != nil {
			logger.Errorf("re-watch stream failed, err: %s, rid: %s", e.Error(), subRid)
			retry.Sleep()
			continue
		}

		logger.Infof("re-watch stream success, rid: %s", subRid)
		break
	}

	// set reconnecting to false.
	w.reconnecting.Store(false)

	logger.Infof("reconnect the upstream server success, rid: %s", rid)
}
