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

package watch

import (
	"fmt"
	"time"

	"bscp.io/pkg/kit"
	sfs "bscp.io/pkg/sf-share"
	"bscp.io/pkg/tools"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/logger"
	"github.com/TencentBlueKing/bscp-go/types"
)

const (
	// TODO: these config can set by config file.
	// defaultHeartbeatIntervalSec defines heartbeat default interval.
	defaultHeartbeatInterval = 15 * time.Second
	// defaultHeartbeatTimeout defines default heartbeat request timeout.
	defaultHeartbeatTimeout = 5 * time.Second
	// maxHeartbeatRetryCount defines heartbeat max retry count.
	maxHeartbeatRetryCount = 3
)

func (w *Watcher) loopHeartbeat() error {

	apps := make([]sfs.SideAppMeta, 0, len(w.subscribers))
	for _, subscriber := range w.subscribers {
		apps = append(apps, sfs.SideAppMeta{
			App:              subscriber.App,
			Labels:           subscriber.Labels,
			Uid:              subscriber.UID,
			CurrentReleaseID: subscriber.CurrentReleaseID,
		})
	}
	heartbeatPayload := sfs.HeartbeatPayload{
		FingerPrint:  w.opts.Fingerprint,
		Applications: apps,
	}
	payload, err := heartbeatPayload.Encode()
	if err != nil {
		logger.Error("stream start loop heartbeat failed, encode heartbeat payload err, %s", err.Error())
		return fmt.Errorf("encode heartbeat payload err, %s", err.Error())
	}

	logger.Info("stream start loop heartbeat", slog.Duration("interval", defaultHeartbeatInterval))

	w.vas.Wg.Add(1)
	go func() {
		defer w.vas.Wg.Done()

		tick := time.NewTicker(defaultHeartbeatInterval)
		defer tick.Stop()

		for {
			select {
			case <-w.vas.Ctx.Done():
				logger.Info("stream heartbeat stoped because of %s", w.vas.Ctx.Err().Error())
				return

			case <-tick.C:
				logger.Debug("stream will heartbeat, rid: %s", w.vas.Rid)

				if err := w.heartbeatOnce(w.vas, heartbeatPayload.MessagingType(), payload); err != nil {
					logger.Warn("stream heartbeat failed, notify reconnect upstream, err: %v, rid: %s", err, w.vas.Rid)

					w.NotifyReconnect(types.ReconnectSignal{Reason: "stream heartbeat failed"})
					return
				}
				logger.Debug("stream heartbeat successfully, rid: %s", w.vas.Rid)
			}
		}
	}()

	return nil
}

// heartbeatOnce send heartbeat to upstream server, if failed maxHeartbeatRetryCount count, return error.
func (w *Watcher) heartbeatOnce(vas *kit.Vas, msgType sfs.MessagingType, payload []byte) error {
	retry := tools.NewRetryPolicy(maxHeartbeatRetryCount, [2]uint{1000, 3000})

	var lastErr error
	for {
		select {
		case <-w.vas.Ctx.Done():
			return nil
		default:
		}

		if retry.RetryCount() == maxHeartbeatRetryCount {
			return lastErr
		}

		if err := w.sendHeartbeatMessaging(vas, msgType, payload); err != nil {
			logger.Error("send heartbeat message failed, retry count: %d, err: %v, rid: %s",
				retry.RetryCount(), err, vas.Rid)
			lastErr = err
			retry.Sleep()
			continue
		}

		return nil
	}
}

// sendHeartbeatMessaging send heartbeat message to upstream server.
func (w *Watcher) sendHeartbeatMessaging(vas *kit.Vas, msgType sfs.MessagingType, payload []byte) error {
	timeoutVas, cancel := vas.WithTimeout(defaultHeartbeatTimeout)
	defer cancel()

	if _, err := w.upstream.Messaging(timeoutVas, msgType, payload); err != nil {
		return err
	}

	return nil
}
