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
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"time"

	"bscp.io/pkg/criteria/constant"
	"bscp.io/pkg/kit"
	"bscp.io/pkg/logs"
	pbfs "bscp.io/pkg/protocol/feed-server"
	"bscp.io/pkg/runtime/jsoni"
	sfs "bscp.io/pkg/sf-share"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/TencentBlueKing/bscp-go/cache"
	"github.com/TencentBlueKing/bscp-go/metrics"
	"github.com/TencentBlueKing/bscp-go/option"
	"github.com/TencentBlueKing/bscp-go/pkg/util"
	"github.com/TencentBlueKing/bscp-go/types"
	"github.com/TencentBlueKing/bscp-go/upstream"
)

// Watcher is the main watch stream for instance
type Watcher struct {
	subscribers     []*Subscriber
	vas             *kit.Vas
	cancel          context.CancelFunc
	opts            option.WatchOptions
	metaHeaderValue string
	reconnectChan   chan types.ReconnectSignal
	reconnecting    *atomic.Bool
	Conn            *grpc.ClientConn
	upstream        upstream.Upstream
}

func (w *Watcher) buildVas() (*kit.Vas, context.CancelFunc) {
	pairs := make(map[string]string)
	// add user information
	pairs[constant.SideUserKey] = "TODO-USER"
	// add finger printer
	pairs[constant.SidecarMetaKey] = w.metaHeaderValue

	vas := kit.OutgoingVas(pairs)
	ctx, cancel := context.WithCancel(vas.Ctx)
	vas.Ctx = ctx
	return vas, cancel
}

// New return a Watcher
func New(u upstream.Upstream, opts option.WatchOptions) (*Watcher, error) {
	w := &Watcher{
		opts:     opts,
		upstream: u,
	}
	mh := sfs.SidecarMetaHeader{
		BizID:       w.opts.BizID,
		Fingerprint: w.opts.Fingerprint,
	}
	mhBytes, err := jsoni.Marshal(mh)
	if err != nil {
		return nil, fmt.Errorf("encode sidecar meta header failed, err: %s", err.Error())
	}
	w.metaHeaderValue = string(mhBytes)
	return w, nil
}

// StartWatch start watch stream
func (w *Watcher) StartWatch() error {
	w.vas, w.cancel = w.buildVas()
	w.reconnectChan = make(chan types.ReconnectSignal, 5)
	w.reconnecting = atomic.NewBool(false)
	var err error
	apps := []sfs.SideAppMeta{}
	for _, subscriber := range w.subscribers {
		apps = append(apps, sfs.SideAppMeta{
			App:              subscriber.App,
			Uid:              subscriber.UID,
			Labels:           subscriber.Labels,
			CurrentReleaseID: subscriber.CurrentReleaseID,
			CurrentCursorID:  0,
		})
	}
	payload := sfs.SideWatchPayload{
		BizID:        w.opts.BizID,
		Applications: apps,
	}
	bytes, err := jsoni.Marshal(payload)
	if err != nil {
		w.cancel()
		return fmt.Errorf("encode watch payload failed, err: %s", err.Error())
	}
	upstreamClient, err := w.upstream.Watch(w.vas, bytes)
	if err != nil {
		w.cancel()
		return fmt.Errorf("watch upstream server with payload failed, err: %s", err.Error())
	}
	go w.waitForReconnectSignal()
	go w.loopReceiveWatchedEvent(upstreamClient)
	if err = w.loopHeartbeat(); err != nil {
		w.cancel()
		return fmt.Errorf("start loop hearbeat failed, err: %s", err.Error())
	}
	return nil
}

func (w *Watcher) loopReceiveWatchedEvent(wStream pbfs.Upstream_WatchClient) {
	w.vas.Wg.Add(1)
	type RecvResult struct {
		event *pbfs.FeedWatchMessage
		err   error
	}
	resultChan := make(chan RecvResult)
	go func() {
		for {
			select {
			case <-w.vas.Ctx.Done():
				logs.Infof("stop receive upstream event because of %s", w.vas.Ctx.Err().Error())
				return
			default:
			}
			event, err := wStream.Recv()
			resultChan <- RecvResult{event, err}
		}
	}()
	for {
		select {
		case <-w.vas.Ctx.Done():
			logs.Warnf("watch will closed because of %s", w.vas.Ctx.Err().Error())

			if err := wStream.CloseSend(); err != nil {
				logs.Errorf("close watch failed, err: %s", err.Error())
				return
			}

			logs.Infof("watch is closed successfully")
			w.vas.Wg.Done()
			return

		case result := <-resultChan:
			event, err := result.event, result.err

			if err != nil {
				if errors.Is(err, io.EOF) {
					logs.Errorf("watch stream has been closed by remote upstream stream server, need to re-connect again")
					w.NotifyReconnect(types.ReconnectSignal{Reason: "connection is closed " +
						"by remote upstream server"})
					return
				}

				logs.Errorf("watch stream is corrupted because of %s, rid: %s", err.Error(), w.vas.Rid)
				w.NotifyReconnect(types.ReconnectSignal{Reason: "watch stream corrupted"})
				return
			}

			logs.Infof("received upstream event, apiVersion: %s, payload: %s, rid: %s", event.ApiVersion.Format(),
				event.Payload, event.Rid)

			if !sfs.IsAPIVersionMatch(event.ApiVersion) {
				// 此处是不是不应该做版本兼容的校验？
				// TODO: set sidecar unhealthy, offline and exit.
				logs.Errorf("watch stream received incompatible event version: %s, rid: %s", event.ApiVersion.Format(),
					event.Rid)
				break
			}

			switch sfs.FeedMessageType(event.Type) {
			case sfs.Bounce:
				logs.Infof("received upstream bounce request, need to reconnect upstream server, rid: %s", event.Rid)
				w.NotifyReconnect(types.ReconnectSignal{Reason: "received bounce request"})
				return

			case sfs.PublishRelease:
				logs.Infof("received upstream publish release event, rid: %s", event.Rid)
				change := &sfs.ReleaseChangeEvent{
					Rid:        event.Rid,
					APIVersion: event.ApiVersion,
					Payload:    event.Payload,
				}

				if c := cache.GetCache(); c != nil {
					go c.OnReleaseChange(change)
				}
				go w.OnReleaseChange(change)
				continue

			default:
				logs.Errorf("watch stream received unsupported event type: %s, skip, rid: %s", event.Type, event.Rid)
				continue
			}
		}
	}
}

// StopWatch close watch stream
func (w *Watcher) StopWatch() {
	if w.cancel == nil {
		return
	}

	w.cancel()

	w.vas.Wg.Wait()
}

// OnReleaseChange handle all instances release change event
func (w *Watcher) OnReleaseChange(event *sfs.ReleaseChangeEvent) {
	// parse payload according the api version.
	pl := new(sfs.ReleaseChangePayload)
	if err := jsoni.Unmarshal(event.Payload, pl); err != nil {
		logs.Errorf("decode release change event payload failed, skip the event, err: %s, rid: %s", err.Error(), event.Rid)
		return
	}
	// TODO: encode subscriber options(App, UID, Labels) to a unique string key
	for _, subscriber := range w.subscribers {
		if subscriber.App == pl.Instance.App &&
			subscriber.UID == pl.Instance.Uid &&
			reflect.DeepEqual(subscriber.Labels, pl.Instance.Labels) &&
			subscriber.CurrentReleaseID != pl.ReleaseMeta.ReleaseID {

			subscriber.CurrentReleaseID = pl.ReleaseMeta.ReleaseID

			// TODO: check if the subscriber watched config items are changed
			// if subscriber.CheckConfigItemsChanged(pl.ReleaseMeta.CIMetas) {
			subscriber.ResetConfigItems(pl.ReleaseMeta.CIMetas)
			// TODO: filter config items by subscriber options
			configItemFiles := []*types.ConfigItemFile{}
			for _, ci := range pl.ReleaseMeta.CIMetas {
				configItemFiles = append(configItemFiles, &types.ConfigItemFile{
					Name:       ci.ConfigItemSpec.Name,
					Path:       ci.ConfigItemSpec.Path,
					Permission: ci.ConfigItemSpec.Permission,
					FileMeta:   ci,
				})
			}
			// TODO: need to retry if callback with error ?
			start := time.Now()
			if err := subscriber.Callback(pl.ReleaseMeta.ReleaseID, configItemFiles,
				pl.ReleaseMeta.PreHook, pl.ReleaseMeta.PostHook); err != nil {
				logs.Errorf("execute watch callback for app %s failed, err: %s", subscriber.App, err.Error())
				subscriber.reportReleaseChangeCallbackMetrics("failed", start)
			}
			subscriber.reportReleaseChangeCallbackMetrics("success", start)
		}
	}
}

// Subscribe subscribe the instance release change event
func (w *Watcher) Subscribe(callback option.Callback, app string, opts ...option.AppOption) *Subscriber {
	options := &option.AppOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if options.UID == "" {
		options.UID = w.opts.Fingerprint
	}
	subscriber := &Subscriber{
		App:  app,
		Opts: options,
		// merge labels, if key conflict, app value will overwrite client value
		Labels:           util.MergeLabels(w.opts.Labels, options.Labels),
		UID:              options.UID,
		Callback:         callback,
		CurrentReleaseID: 0,
	}
	w.subscribers = append(w.subscribers, subscriber)
	return subscriber
}

// Subscribers return all subscribers
func (w *Watcher) Subscribers() []*Subscriber {
	return w.subscribers
}

// Subscriber is the subscriber of the instance
type Subscriber struct {
	Opts *option.AppOptions
	App  string
	// Callback is the callback function when the watched items are changed
	Callback option.Callback
	// CurrentReleaseID is the current release id of the subscriber
	CurrentReleaseID uint32
	// Labels is the labels of the subscriber
	Labels map[string]string
	// UID is the unique id of the subscriber
	UID string
	// currentConfigItems store the current config items of the subscriber, map[configItemName]commitID
	currentConfigItems map[string]uint32
}

// CheckConfigItemsChanged check if the subscriber watched config items are changed
// Deprecated: commit id can not be used to check config items changed anymore
// ? Should it used in file mode ?
func (s *Subscriber) CheckConfigItemsChanged(cis []*sfs.ConfigItemMetaV1) bool {
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
func (s *Subscriber) ResetConfigItems(cis []*sfs.ConfigItemMetaV1) {
	// TODO: Filter by watch options(pattern/regex)
	m := make(map[string]uint32)
	for _, ci := range cis {
		m[ci.ConfigItemSpec.Name] = ci.CommitID
	}
	s.currentConfigItems = m
}

// ResetLabels reset the labels of the subscriber
// s.Opts.Labels as origion labels would not be reset
func (s *Subscriber) ResetLabels(labels map[string]string) {
	s.Labels = util.MergeLabels(labels, s.Opts.Labels)
}

func (s *Subscriber) reportReleaseChangeCallbackMetrics(status string, start time.Time) {
	releaseID := strconv.Itoa(int(s.CurrentReleaseID))
	metrics.ReleaseChangeCallbackCounter.WithLabelValues(s.App, status, releaseID).Inc()
	seconds := time.Since(start).Seconds()
	metrics.ReleaseChangeCallbackHandingSecond.WithLabelValues(s.App, status, releaseID).Observe(seconds)
}
