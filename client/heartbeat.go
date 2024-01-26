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
	"time"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/tools"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/internal/util/host"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
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

func (w *watcher) loopHeartbeat() error { // nolint
	logger.Info("stream start loop heartbeat", slog.Duration("interval", defaultHeartbeatInterval))
	var (
		maxMemoryUsage, currentMemoryUsage uint64
		maxCPUUsage, currentCPUUsage       float64
	)
	w.vas.Wg.Add(1)
	go func() {
		defer w.vas.Wg.Done()

		tick := time.NewTicker(defaultHeartbeatInterval)
		defer tick.Stop()

		for {
			select {
			case <-w.vas.Ctx.Done():
				logger.Info("stream heartbeat stoped because of ctx done", logger.ErrAttr(w.vas.Ctx.Err()))
				return

			case <-tick.C:
				logger.Debug("stream will heartbeat", slog.String("rid", w.vas.Rid))

				apps := make([]sfs.SideAppMeta, 0, len(w.subscribers))
				for _, subscriber := range w.subscribers {
					apps = append(apps, sfs.SideAppMeta{
						App:                 subscriber.App,
						Labels:              subscriber.Labels,
						Uid:                 subscriber.UID,
						CurrentReleaseID:    subscriber.CurrentReleaseID,
						TargetReleaseID:     subscriber.TargetReleaseID,
						CursorID:            subscriber.CursorID,
						ReleaseChangeStatus: subscriber.ReleaseChangeStatus,
						DownloadFileNum:     subscriber.DownloadFileNum,
						DownloadFileSize:    subscriber.DownloadFileSize,
					})
				}
				heartbeatPayload := sfs.HeartbeatPayload{
					BasicData: sfs.BasicData{
						FingerPrint:   w.opts.fingerprint,
						BizID:         w.opts.bizID,
						HeartbeatTime: time.Now(),
						OnlineStatus:  sfs.Online,
						ClientMode:    w.opts.mode,
						ClientType:    sfs.ClientType(version.CLIENTTYPE),
					},
					Applications: apps,
				}
				currentCPUUsage, maxCPUUsage = host.GetCpuUsage()
				currentMemoryUsage, maxMemoryUsage = host.GetMemUsage()
				heartbeatPayload.ResourceUsage = sfs.ResourceUsage{
					CpuMaxUsage:    maxCPUUsage,
					CpuUsage:       currentCPUUsage,
					MemoryMaxUsage: maxMemoryUsage,
					MemoryUsage:    currentMemoryUsage,
				}
				payload, err := heartbeatPayload.Encode()
				if err != nil {
					logger.Error("stream start loop heartbeat failed by encode heartbeat payload", logger.ErrAttr(err))
					return
				}

				if err := w.heartbeatOnce(w.vas, heartbeatPayload.MessagingType(), payload); err != nil {
					logger.Warn("stream heartbeat failed, notify reconnect upstream",
						logger.ErrAttr(err), slog.String("rid", w.vas.Rid))

					w.NotifyReconnect(reconnectSignal{Reason: "stream heartbeat failed"})
					return
				}
				logger.Debug("stream heartbeat successfully", slog.String("rid", w.vas.Rid))
			}
		}
	}()

	return nil
}

// heartbeatOnce send heartbeat to upstream server, if failed maxHeartbeatRetryCount count, return error.
func (w *watcher) heartbeatOnce(vas *kit.Vas, msgType sfs.MessagingType, payload []byte) error {
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
			logger.Error("send heartbeat message failed",
				slog.Any("retry_count", retry.RetryCount()), logger.ErrAttr(err), slog.String("rid", vas.Rid))
			lastErr = err
			retry.Sleep()
			continue
		}

		return nil
	}
}

// sendHeartbeatMessaging send heartbeat message to upstream server.
func (w *watcher) sendHeartbeatMessaging(vas *kit.Vas, msgType sfs.MessagingType, payload []byte) error {
	timeoutVas, cancel := vas.WithTimeout(defaultHeartbeatTimeout)
	defer cancel()

	if _, err := w.upstream.Messaging(timeoutVas, msgType, payload); err != nil {
		return err
	}

	return nil
}

// Heartbeat xxx
type Heartbeat struct {
	subscribers     *sfs.SideAppMeta
	upstream        upstream.Upstream
	opts            *options
	CurrentCursorID uint32
	vas             *kit.Vas
}

// NewHeartbeat xxx
func NewHeartbeat(vas *kit.Vas, upstream upstream.Upstream, opts options, cursorID string) *Heartbeat {
	return &Heartbeat{
		subscribers: &sfs.SideAppMeta{
			CursorID: cursorID,
			Uid:      opts.uid,
			Labels:   opts.labels,
		},
		vas:      vas,
		upstream: upstream,
		opts:     &opts,
	}
}

// pull时定时上报心跳
func (h *Heartbeat) loopHeartbeat() {
	var (
		maxMemoryUsage, currentMemoryUsage uint64
		maxCPUUsage, currentCPUUsage       float64
	)

	go func() {
		tick := time.NewTicker(defaultHeartbeatInterval)
		defer tick.Stop()
		for {
			select {
			case <-h.vas.Ctx.Done():
				logger.Info("stream heartbeat stoped because of ctx done")
				return
			case <-tick.C:
				apps := make([]sfs.SideAppMeta, 0)
				apps = append(apps, *h.subscribers)

				heartbeatPayload := sfs.HeartbeatPayload{
					BasicData: sfs.BasicData{
						FingerPrint:   h.opts.fingerprint,
						BizID:         h.opts.bizID,
						HeartbeatTime: time.Now(),
						OnlineStatus:  sfs.Online,
						ClientMode:    h.opts.mode,
						ClientType:    sfs.ClientType(version.CLIENTTYPE),
					},
					Applications: apps,
				}
				currentCPUUsage, maxCPUUsage = host.GetCpuUsage()
				currentMemoryUsage, maxMemoryUsage = host.GetMemUsage()
				heartbeatPayload.ResourceUsage = sfs.ResourceUsage{
					CpuMaxUsage:    maxCPUUsage,
					CpuUsage:       currentCPUUsage,
					MemoryMaxUsage: maxMemoryUsage,
					MemoryUsage:    currentMemoryUsage,
				}
				payload, err := heartbeatPayload.Encode()
				if err != nil {
					logger.Error("stream start loop heartbeat failed by encode heartbeat payload", logger.ErrAttr(err))
					return
				}

				if err := h.heartbeatOnce(heartbeatPayload.MessagingType(), payload); err != nil {
					logger.Warn("stream heartbeat failed, notify reconnect upstream",
						logger.ErrAttr(err), slog.String("rid", h.vas.Rid))
					return
				}
				logger.Debug("stream heartbeat successfully", slog.String("rid", h.vas.Rid))
			}
		}
	}()
}

// GetApplication xxx
func (h *Heartbeat) GetApplication() *sfs.SideAppMeta {
	return h.subscribers
}

// heartbeatOnce send heartbeat to upstream server, if failed maxHeartbeatRetryCount count, return error.
func (h *Heartbeat) heartbeatOnce(msgType sfs.MessagingType, payload []byte) error {
	retry := tools.NewRetryPolicy(maxHeartbeatRetryCount, [2]uint{1000, 3000})

	var lastErr error
	for {
		select {
		case <-h.vas.Ctx.Done():
			return nil
		default:
		}

		if retry.RetryCount() == maxHeartbeatRetryCount {
			return lastErr
		}

		if err := h.sendHeartbeatMessaging(h.vas, msgType, payload); err != nil {
			logger.Error("send heartbeat message failed",
				slog.Any("retry_count", retry.RetryCount()), logger.ErrAttr(err), slog.String("rid", h.vas.Rid))
			lastErr = err
			retry.Sleep()
			continue
		}

		return nil
	}
}

// sendHeartbeatMessaging send heartbeat message to upstream server.
func (h *Heartbeat) sendHeartbeatMessaging(vas *kit.Vas, msgType sfs.MessagingType, payload []byte) error {
	timeoutVas, cancel := vas.WithTimeout(defaultHeartbeatTimeout)
	defer cancel()

	if _, err := h.upstream.Messaging(timeoutVas, msgType, payload); err != nil {
		return err
	}

	return nil
}
