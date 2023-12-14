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

// Package client defines the bscp-go client.
package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/criteria/constant"
	"github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	pbfs "github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/feed-server"
	sfs "github.com/TencentBlueking/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"golang.org/x/exp/slog"

	"github.com/TencentBlueKing/bscp-go/internal/cache"
	"github.com/TencentBlueKing/bscp-go/internal/downloader"
	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/internal/watch"
	"github.com/TencentBlueKing/bscp-go/logger"
	"github.com/TencentBlueKing/bscp-go/types"
)

// Client bscp client method
type Client interface {
	// PullFiles pull files from remote
	PullFiles(app string, opts ...types.AppOption) (*types.Release, error)
	// Pull Key Value from remote
	Get(app string, key string, opts ...types.AppOption) (string, error)
	// AddWatcher add a watcher to client
	AddWatcher(callback types.Callback, app string, opts ...types.AppOption) error
	// StartWatch start watch
	StartWatch() error
	// StopWatch stop watch
	StopWatch()
	// ResetLabels reset bscp client labels, if key conflict, app value will overwrite client value
	ResetLabels(labels map[string]string)
}

// Client is the bscp client
type client struct {
	pairs       map[string]string
	opts        ClientOptions
	fingerPrint sfs.FingerPrint
	watcher     *watch.Watcher
	upstream    upstream.Upstream
}

// New return a bscp client instance
func New(opts ...ClientOption) (Client, error) {
	clientOpt := &ClientOptions{}
	fp, err := sfs.GetFingerPrint()
	if err != nil {
		return nil, fmt.Errorf("get instance fingerprint failed, err: %s", err.Error())
	}
	logger.Info("instance fingerprint", slog.String("fingerprint", fp.Encode()))
	clientOpt.Fingerprint = fp.Encode()
	clientOpt.UID = clientOpt.Fingerprint
	for _, opt := range opts {
		if e := opt(clientOpt); e != nil {
			return nil, e
		}
	}
	// prepare pairs
	pairs := make(map[string]string)
	// add user information
	pairs[constant.SideUserKey] = "TODO-USER"
	// add finger printer
	mh := sfs.SidecarMetaHeader{
		BizID:       clientOpt.BizID,
		Fingerprint: clientOpt.Fingerprint,
	}
	mhBytes, err := json.Marshal(mh)
	if err != nil {
		return nil, fmt.Errorf("encode sidecar meta header failed, err: %s", err.Error())
	}
	pairs[constant.SidecarMetaKey] = string(mhBytes)
	// prepare upstream
	u, err := upstream.New(
		upstream.WithFeedAddrs(clientOpt.FeedAddrs),
		upstream.WithDialTimeoutMS(clientOpt.DialTimeoutMS),
		upstream.WithBizID(clientOpt.BizID))
	if err != nil {
		return nil, fmt.Errorf("init upstream client failed, err: %s", err.Error())
	}
	c := &client{
		opts:        *clientOpt,
		fingerPrint: fp,
		upstream:    u,
		pairs:       pairs,
	}
	// handshake
	vas, _ := c.buildVas()
	msg := &pbfs.HandshakeMessage{
		ApiVersion: sfs.CurrentAPIVersion,
		Spec: &pbfs.SidecarSpec{
			BizId:   clientOpt.BizID,
			Version: c.upstream.Version(),
		},
	}
	resp, err := c.upstream.Handshake(vas, msg)
	if err != nil {
		return nil, fmt.Errorf("handshake with upstream failed, err: %s, rid: %s", err.Error(), vas.Rid)
	}
	pl := &sfs.SidecarHandshakePayload{}
	err = json.Unmarshal(resp.Payload, pl)
	if err != nil {
		return nil, fmt.Errorf("decode handshake payload failed, err: %s, rid: %s", err.Error(), vas.Rid)
	}
	err = downloader.Init(vas, clientOpt.BizID, clientOpt.Token, u, pl.RuntimeOption.RepositoryTLS)
	if err != nil {
		return nil, fmt.Errorf("init downloader failed, err: %s", err.Error())
	}
	if clientOpt.UseFileCache {
		cache.Init(true, clientOpt.FileCacheDir)
	}
	watcher, err := watch.New(u, watch.WatchOptions{
		BizID:       clientOpt.BizID,
		Labels:      clientOpt.Labels,
		Fingerprint: fp.Encode(),
	})
	if err != nil {
		return nil, fmt.Errorf("init watcher failed, err: %s", err.Error())
	}
	c.watcher = watcher
	return c, nil
}

// AddWatcher add a watcher to client
func (c *client) AddWatcher(callback types.Callback, app string, opts ...types.AppOption) error {
	_ = c.watcher.Subscribe(callback, app, opts...)
	return nil
}

// StartWatch start watch
func (c *client) StartWatch() error {
	return c.watcher.StartWatch()
}

// StopWatch stop watch
func (c *client) StopWatch() {
	c.watcher.StopWatch()
}

// ResetLabels reset bscp client labels, if key conflict, app value will overwrite client value
func (c *client) ResetLabels(labels map[string]string) {
	c.opts.Labels = labels
	for _, subscriber := range c.watcher.Subscribers() {
		subscriber.ResetLabels(labels)
	}

	c.watcher.NotifyReconnect(watch.ReconnectSignal{Reason: "reset labels"})
}

// PullFiles pull files from remote
func (c *client) PullFiles(app string, opts ...types.AppOption) (*types.Release, error) {
	option := &types.AppOptions{}
	for _, opt := range opts {
		opt(option)
	}
	vas, _ := c.buildVas()
	req := &pbfs.PullAppFileMetaReq{
		ApiVersion: sfs.CurrentAPIVersion,
		BizId:      c.opts.BizID,
		AppMeta: &pbfs.AppMeta{
			App:    app,
			Labels: c.opts.Labels,
			Uid:    c.opts.UID,
		},
		Token: c.opts.Token,
		Key:   option.Key,
	}
	// merge labels, if key conflict, app value will overwrite client value
	req.AppMeta.Labels = util.MergeLabels(c.opts.Labels, option.Labels)
	// reset uid
	if option.UID != "" {
		req.AppMeta.Uid = option.UID
	}
	resp, err := c.upstream.PullAppFileMeta(vas, req)
	if err != nil {
		return nil, fmt.Errorf("pull file meta failed, err: %s, rid: %s", err.Error(), vas.Rid)
	}
	files := make([]*types.ConfigItemFile, len(resp.FileMetas))
	for i, meta := range resp.FileMetas {
		files[i] = &types.ConfigItemFile{
			Name:       meta.ConfigItemSpec.Name,
			Path:       meta.ConfigItemSpec.Path,
			Permission: meta.ConfigItemSpec.Permission,
			FileMeta: &sfs.ConfigItemMetaV1{
				ID:                   meta.Id,
				CommitID:             meta.CommitId,
				ContentSpec:          meta.CommitSpec.Content,
				ConfigItemSpec:       meta.ConfigItemSpec,
				ConfigItemAttachment: meta.ConfigItemAttachment,
				RepositoryPath:       meta.RepositorySpec.Path,
			},
		}
	}

	r := &types.Release{
		ReleaseID: resp.ReleaseId,
		FileItems: files,
		PreHook:   resp.PreHook,
		PostHook:  resp.PostHook,
	}
	return r, nil
}

// Get 读取 Key 的值
func (c *client) Get(app string, key string, opts ...types.AppOption) (string, error) {
	option := &types.AppOptions{}
	for _, opt := range opts {
		opt(option)
	}
	vas, _ := c.buildVas()
	req := &pbfs.GetKvValueReq{
		ApiVersion: sfs.CurrentAPIVersion,
		BizId:      c.opts.BizID,
		AppMeta: &pbfs.AppMeta{
			App:    app,
			Labels: c.opts.Labels,
			Uid:    c.opts.UID,
		},
		Token: c.opts.Token,
		Key:   key,
	}
	req.AppMeta.Labels = util.MergeLabels(c.opts.Labels, option.Labels)
	// reset uid
	if option.UID != "" {
		req.AppMeta.Uid = option.UID
	}
	resp, err := c.upstream.GetKvValue(vas, req)
	if err != nil {
		return "", fmt.Errorf("get kv value failed, err: %s, rid: %s", err, vas.Rid)
	}

	return resp.Value, nil
}

func (c *client) buildVas() (*kit.Vas, context.CancelFunc) { // nolint
	vas := kit.OutgoingVas(c.pairs)
	ctx, cancel := context.WithCancel(vas.Ctx)
	vas.Ctx = ctx
	return vas, cancel
}
