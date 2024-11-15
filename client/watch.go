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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/criteria/constant"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	pbfs "github.com/TencentBlueKing/bk-bscp/pkg/protocol/feed-server"
	sfs "github.com/TencentBlueKing/bk-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bscp/pkg/version"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"

	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/internal/util/process_collect"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
	"github.com/TencentBlueKing/bscp-go/pkg/metrics"
)

// ReconnectSignal defines the signal information to tell the
// watcher to reconnect the remote upstream server.
type reconnectSignal struct {
	Reason string
}

// String format the reconnect signal to a string.
func (rs reconnectSignal) String() string {
	return rs.Reason
}

// Watcher is the main watch stream for instance
type watcher struct {
	subscribers     []*subscriber
	vas             *kit.Vas
	cancel          context.CancelFunc
	opts            *options
	metaHeaderValue string
	reconnectChan   chan reconnectSignal
	Conn            *grpc.ClientConn
	upstream        upstream.Upstream
}

func (w *watcher) buildVas() (*kit.Vas, context.CancelFunc) {
	pairs := make(map[string]string)
	// add finger printer
	pairs[constant.SidecarMetaKey] = w.metaHeaderValue

	vas := kit.OutgoingVas(pairs)
	ctx, cancel := context.WithCancel(vas.Ctx)
	vas.Ctx = ctx
	return vas, cancel
}

// New return a Watcher
func newWatcher(u upstream.Upstream, opts *options) (*watcher, error) {
	w := &watcher{
		opts:     opts,
		upstream: u,
		// 重启按原子顺序, 添加一个buff, 对labelfile watch的场景，保留一个重启次数
		reconnectChan: make(chan reconnectSignal, 1),
	}

	mh := sfs.SidecarMetaHeader{
		BizID:       w.opts.bizID,
		Fingerprint: w.opts.fingerprint,
	}
	mhBytes, err := json.Marshal(mh)
	if err != nil {
		return nil, fmt.Errorf("encode sidecar meta header failed, err: %s", err.Error())
	}
	w.metaHeaderValue = string(mhBytes)
	return w, nil
}

// StartWatch start watch stream
func (w *watcher) StartWatch() error {
	w.vas, w.cancel = w.buildVas()

	var err error
	apps := []sfs.SideAppMeta{}
	for _, subscriber := range w.subscribers {
		apps = append(apps, sfs.SideAppMeta{
			App:              subscriber.App,
			Uid:              subscriber.UID,
			Labels:           subscriber.Labels,
			Match:            subscriber.Match,
			CurrentReleaseID: subscriber.CurrentReleaseID,
			CurrentCursorID:  0,
		})
	}
	payload := sfs.SideWatchPayload{
		BizID:        w.opts.bizID,
		Applications: apps,
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		w.cancel()
		return fmt.Errorf("encode watch payload failed, err: %s", err.Error())
	}
	upstreamClient, err := w.upstream.Watch(w.vas, bytes)
	if err != nil {
		w.cancel()
		return fmt.Errorf("watch upstream server with payload failed, err: %s", err.Error())
	}

	// Determine whether to collect resources
	if w.opts.enableMonitorResourceUsage {
		go process_collect.NewProcessCollector(w.vas.Ctx)
	}

	// 发送客户端连接信息
	go func() {
		if err = w.sendClientMessaging(apps, nil); err != nil {
			logger.Error("failed to send the client connection event",
				slog.Uint64("biz", uint64(w.opts.bizID)), logger.ErrAttr(err))
		}
	}()

	go w.waitForReconnectSignal()

	w.vas.Wg.Add(1)
	go func() {
		defer w.vas.Wg.Done()
		w.loopReceiveWatchedEvent(upstreamClient)
	}()

	if err = w.loopHeartbeat(); err != nil {
		w.cancel()
		return fmt.Errorf("start loop hearbeat failed, err: %s", err.Error())
	}
	return nil
}

// StopWatch close watch stream
func (w *watcher) StopWatch() {
	st := time.Now()
	if w.cancel == nil {
		return
	}

	w.cancel()

	w.vas.Wg.Wait()
	logger.Info("stop watch done", slog.String("rid", w.vas.Rid), slog.Duration("duration", time.Since(st)))
}

func (w *watcher) loopReceiveWatchedEvent(wStream pbfs.Upstream_WatchClient) {
	type RecvResult struct {
		event *pbfs.FeedWatchMessage
		err   error
	}

	resultChan := make(chan RecvResult)
	go func() {
		for {
			event, err := wStream.Recv()
			select {
			case <-w.vas.Ctx.Done():
				logger.Info("stop receive upstream event because of ctx is done", logger.ErrAttr(err))
				return
			case resultChan <- RecvResult{event, err}:
			}

		}
	}()
	defer func() {
		if err := wStream.CloseSend(); err != nil {
			logger.Error("close watch stream failed", logger.ErrAttr(err))
		}
	}()

	for {
		select {
		case <-w.vas.Ctx.Done():
			logger.Info("watch stream will be closed because of ctx done", logger.ErrAttr(w.vas.Ctx.Err()))
			return

		case result := <-resultChan:
			event, err := result.event, result.err

			if err != nil {
				if errors.Is(err, io.EOF) {
					logger.Error("watch stream has been closed by remote upstream stream server, need to re-connect again")
					w.NotifyReconnect(reconnectSignal{Reason: "connection is closed " +
						"by remote upstream server"})
					return
				}

				logger.Error("watch stream is corrupted", logger.ErrAttr(err), slog.String("rid", w.vas.Rid))
				// 权限不足或者删除等会一直错误，限制重连频率
				time.Sleep(time.Second * 5)
				w.NotifyReconnect(reconnectSignal{Reason: "watch stream corrupted"})
				return
			}

			logger.Debug("received upstream event",
				slog.String("apiVersion", event.ApiVersion.Format()),
				slog.Any("payload", event.Payload),
				slog.String("rid", event.Rid))

			if !sfs.IsAPIVersionMatch(event.ApiVersion) {
				// 此处是不是不应该做版本兼容的校验？
				// TODO: set sidecar unhealthy, offline and exit.
				logger.Error("watch stream received incompatible event",
					slog.String("version", event.ApiVersion.Format()),
					slog.String("rid", event.Rid))
				break
			}

			switch sfs.FeedMessageType(event.Type) {
			case sfs.Bounce:
				logger.Info("received upstream bounce request, need to reconnect upstream server", slog.String("rid", event.Rid))
				w.NotifyReconnect(reconnectSignal{Reason: "received bounce request"})
				return

			case sfs.PublishRelease:
				logger.Info("received upstream publish release event", slog.String("rid", event.Rid))
				change := &sfs.ReleaseChangeEvent{
					Rid:        event.Rid,
					APIVersion: event.ApiVersion,
					Payload:    event.Payload,
				}

				go w.OnReleaseChange(change)
				continue

			default:
				logger.Error("watch stream received unsupported event, skip",
					slog.Any("type", event.Type), slog.String("rid", event.Rid))
				continue
			}
		}
	}
}

// OnReleaseChange handle all instances release change event
func (w *watcher) OnReleaseChange(event *sfs.ReleaseChangeEvent) { // nolint
	// parse payload according the api version.
	pl := new(sfs.ReleaseChangePayload)
	if err := json.Unmarshal(event.Payload, pl); err != nil {
		logger.Error("decode release change event payload failed, skip the event",
			logger.ErrAttr(err), slog.String("rid", event.Rid))
		return
	}
	// 如果事件ID为0根据 bizID+当前时间生成 事件ID
	var cursorID string
	if pl.CursorID == 0 {
		cursorID = util.GenerateCursorID(w.opts.bizID)
	} else {
		cursorID = strconv.FormatUint(uint64(pl.CursorID), 10)
	}

	// TODO: encode subscriber options(App, UID, Labels) to a unique string key
	for _, subscriber := range w.subscribers {
		if subscriber.App == pl.Instance.App &&
			subscriber.UID == pl.Instance.Uid &&
			reflect.DeepEqual(subscriber.Labels, pl.Instance.Labels) {

			// 更新心跳数据需要cursorID
			subscriber.CursorID = cursorID

			// TODO: check if the subscriber watched config items are changed
			// if subscriber.CheckConfigItemsChanged(pl.ReleaseMeta.CIMetas) {
			subscriber.ResetConfigItems(pl.ReleaseMeta.CIMetas)
			// TODO: filter config items by subscriber options
			configItemFiles := []*ConfigItemFile{}
			// 计算总文件大小和总文件数
			var totalFileSize uint64
			for _, ci := range pl.ReleaseMeta.CIMetas {
				ci.ConfigItemSpec.Path = filepath.FromSlash(ci.ConfigItemSpec.Path)
				configItemFiles = append(configItemFiles, &ConfigItemFile{
					Name:          ci.ConfigItemSpec.Name,
					Path:          ci.ConfigItemSpec.Path,
					TextLineBreak: w.opts.textLineBreak,
					Permission:    ci.ConfigItemSpec.Permission,
					FileMeta:      ci,
				})
				totalFileSize += ci.ContentSpec.ContentSpec().ByteSize
			}

			release := &Release{
				ReleaseID:   pl.ReleaseMeta.ReleaseID,
				ReleaseName: pl.ReleaseMeta.ReleaseName,
				FileItems:   configItemFiles,
				KvItems:     pl.ReleaseMeta.KvMetas,
				PreHook:     pl.ReleaseMeta.PreHook,
				PostHook:    pl.ReleaseMeta.PostHook,
				vas:         w.vas,
				upstream:    w.upstream,
				BizID:       w.opts.bizID,
				CursorID:    cursorID,
				ClientMode:  sfs.Watch,
				SemaphoreCh: make(chan struct{}),
				AppMate: &sfs.SideAppMeta{
					App:              subscriber.App,
					Uid:              subscriber.UID,
					Labels:           subscriber.Labels,
					Match:            subscriber.Match,
					CurrentReleaseID: subscriber.CurrentReleaseID,
					TargetReleaseID:  pl.ReleaseMeta.ReleaseID,
					TotalFileSize:    totalFileSize,
					TotalFileNum:     len(configItemFiles),
				},
			}

			start := time.Now()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func(ctx context.Context) {
				for {
					select {
					case <-ctx.Done():
						return
					case <-release.SemaphoreCh:
						successDownloads := atomic.LoadInt32(&release.AppMate.DownloadFileNum)
						successFileSize := atomic.LoadUint64(&release.AppMate.DownloadFileSize)
						subscriber.DownloadFileNum = successDownloads
						subscriber.DownloadFileSize = successFileSize
					}
				}
			}(ctx)

			subscriber.ReleaseChangeStatus = sfs.Processing
			if err := subscriber.Callback(release); err != nil {
				cancel()
				subscriber.ReleaseChangeStatus = sfs.Failed
				logger.Error("execute watch callback failed", slog.String("app", subscriber.App), logger.ErrAttr(err))
				subscriber.reportReleaseChangeCallbackMetrics("failed", start)
			} else {
				cancel()
				subscriber.ReleaseChangeStatus = sfs.Success
				subscriber.reportReleaseChangeCallbackMetrics("success", start)

				subscriber.CurrentReleaseID = pl.ReleaseMeta.ReleaseID
			}
		}
	}
}

// Subscribe subscribe the instance release change event
func (w *watcher) Subscribe(callback Callback, app string, opts ...AppOption) *subscriber {
	options := &AppOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if options.UID == "" {
		options.UID = w.opts.fingerprint
	}
	subscriber := &subscriber{
		App:  app,
		Opts: options,
		// merge labels, if key conflict, app value will overwrite client value
		Labels:           util.MergeLabels(w.opts.labels, options.Labels),
		UID:              options.UID,
		Match:            options.Match,
		Callback:         callback,
		CurrentReleaseID: 0,
	}
	w.subscribers = append(w.subscribers, subscriber)
	return subscriber
}

// Subscribers return all subscribers
func (w *watcher) Subscribers() []*subscriber {
	return w.subscribers
}

// Subscriber is the subscriber of the instance
type subscriber struct {
	Opts *AppOptions
	App  string
	// Callback is the callback function when the watched items are changed
	Callback Callback
	// CurrentReleaseID is the current release id of the subscriber
	CurrentReleaseID uint32
	// TargetReleaseID is sidecar's target release id
	TargetReleaseID uint32
	// Labels is the labels of the subscriber
	Labels map[string]string
	// UID is the unique id of the subscriber
	UID string
	// Match is app config item's match condition
	Match []string
	// currentConfigItems store the current config items of the subscriber, map[configItemName]commitID
	currentConfigItems map[string]uint32
	// CursorID 事件ID
	CursorID string
	// ReleaseChangeStatus 变更状态
	ReleaseChangeStatus sfs.Status
	DownloadFileNum     int32
	DownloadFileSize    uint64
}

// CheckConfigItemsChanged check if the subscriber watched config items are changed
// Deprecated: commit id can not be used to check config items changed anymore
// ? Should it used in file mode ?
func (s *subscriber) CheckConfigItemsChanged(cis []*sfs.ConfigItemMetaV1) bool {
	if len(cis) == 0 {
		return false
	}
	checked := 0
	// TODO: Filter by watch options(pattern/regex)
	for _, ci := range cis {
		commitID, ok := s.currentConfigItems[ci.ConfigItemSpec.Name]
		if !ok || commitID != ci.CommitID {
			return true
		}
		checked++
	}
	// reverse check for confit items deleted event
	return checked != len(s.currentConfigItems)
}

// ResetConfigItems reset the current config items of the subscriber
// Deprecated: commit id can not be used to check config items changed anymore
func (s *subscriber) ResetConfigItems(cis []*sfs.ConfigItemMetaV1) {
	// TODO: Filter by watch options(pattern/regex)
	m := make(map[string]uint32)
	for _, ci := range cis {
		m[ci.ConfigItemSpec.Name] = ci.CommitID
	}
	s.currentConfigItems = m
}

// ResetLabels reset the labels of the subscriber
// s.Opts.Labels as origion labels would not be reset
func (s *subscriber) ResetLabels(labels map[string]string) {
	s.Labels = util.MergeLabels(labels, s.Opts.Labels)
}

func (s *subscriber) reportReleaseChangeCallbackMetrics(status string, start time.Time) {
	releaseID := strconv.Itoa(int(s.TargetReleaseID))
	metrics.ReleaseChangeCallbackCounter.WithLabelValues(s.App, status, releaseID).Inc()
	seconds := time.Since(start).Seconds()
	metrics.ReleaseChangeCallbackHandingSecond.WithLabelValues(s.App, status, releaseID).Observe(seconds)
}

// sendClientMessaging 发送客户端连接信息
func (w *watcher) sendClientMessaging(meta []sfs.SideAppMeta, annotations map[string]interface{}) error {
	clientInfoPayload := sfs.HeartbeatPayload{
		BasicData: sfs.BasicData{
			BizID:         w.opts.bizID,
			ClientMode:    sfs.Watch,
			ClientVersion: version.Version().Version,
			ClientType:    sfs.ClientType(version.CLIENTTYPE),
			IP:            util.GetClientIP(),
			Annotations:   annotations,
		},
		Applications: meta,
	}

	payload, err := clientInfoPayload.Encode()
	if err != nil {
		return err
	}

	_, err = w.upstream.Messaging(w.vas, clientInfoPayload.MessagingType(), payload)
	if err != nil {
		return err
	}
	return nil
}
