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
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

// Client bscp client method
type Client interface {
	// ListApps list app from remote, only return have perm by token
	ListApps(match []string) ([]*pbfs.App, error)
	// PullFiles pull files from remote
	PullFiles(app string, opts ...AppOption) (*Release, error)
	// Get KV release from remote
	PullKvs(app string, match []string, opts ...AppOption) (*Release, error)
	// Pull Key Value from remote
	Get(app string, key string, opts ...AppOption) (string, error)
	// AddWatcher add a watcher to client
	AddWatcher(callback Callback, app string, opts ...AppOption) error
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
	opts        options
	fingerPrint sfs.FingerPrint
	watcher     *watcher
	upstream    upstream.Upstream
}

// New return a bscp client instance
func New(opts ...Option) (Client, error) {
	clientOpt := &options{}
	fp, err := sfs.GetFingerPrint()
	if err != nil {
		return nil, fmt.Errorf("get instance fingerprint failed, err: %s", err.Error())
	}
	logger.Info("instance fingerprint", slog.String("fingerprint", fp.Encode()))
	clientOpt.fingerprint = fp.Encode()
	clientOpt.uid = clientOpt.fingerprint
	for _, opt := range opts {
		if e := opt(clientOpt); e != nil {
			return nil, e
		}
	}
	// prepare pairs
	pairs := make(map[string]string)
	// add user information
	pairs[constant.SideUserKey] = "TODO-USER"

	// 添加头部认证信息
	pairs[authorizationHeader] = bearerKey + " " + clientOpt.token

	// add finger printer
	mh := sfs.SidecarMetaHeader{
		BizID:       clientOpt.bizID,
		Fingerprint: clientOpt.fingerprint,
	}
	mhBytes, err := json.Marshal(mh)
	if err != nil {
		return nil, fmt.Errorf("encode sidecar meta header failed, err: %s", err.Error())
	}
	pairs[constant.SidecarMetaKey] = string(mhBytes)
	// prepare upstream
	u, err := upstream.New(
		upstream.WithFeedAddrs(clientOpt.feedAddrs),
		upstream.WithDialTimeoutMS(clientOpt.dialTimeoutMS),
		upstream.WithBizID(clientOpt.bizID))
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
			BizId:   clientOpt.bizID,
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
	err = downloader.Init(vas, clientOpt.bizID, clientOpt.token, u, pl.RuntimeOption.RepositoryTLS)
	if err != nil {
		return nil, fmt.Errorf("init downloader failed, err: %s", err.Error())
	}
	if clientOpt.useFileCache {
		cache.Init(true, clientOpt.fileCacheDir)
	}
	watcher, err := newWatcher(u, clientOpt)
	if err != nil {
		return nil, fmt.Errorf("init watcher failed, err: %s", err.Error())
	}
	c.watcher = watcher
	return c, nil
}

// AddWatcher add a watcher to client
func (c *client) AddWatcher(callback Callback, app string, opts ...AppOption) error {
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
	c.opts.labels = labels
	for _, subscriber := range c.watcher.Subscribers() {
		subscriber.ResetLabels(labels)
	}

	c.watcher.NotifyReconnect(reconnectSignal{Reason: "reset labels"})
}

// PullFiles pull files from remote
func (c *client) PullFiles(app string, opts ...AppOption) (*Release, error) {
	option := &AppOptions{}
	for _, opt := range opts {
		opt(option)
	}
	vas, _ := c.buildVas()
	req := &pbfs.PullAppFileMetaReq{
		ApiVersion: sfs.CurrentAPIVersion,
		BizId:      c.opts.bizID,
		AppMeta: &pbfs.AppMeta{
			App:    app,
			Labels: c.opts.labels,
			Uid:    c.opts.uid,
		},
		Token: c.opts.token,
		Key:   option.Key,
	}
	// merge labels, if key conflict, app value will overwrite client value
	req.AppMeta.Labels = util.MergeLabels(c.opts.labels, option.Labels)
	// reset uid
	if option.UID != "" {
		req.AppMeta.Uid = option.UID
	}
	resp, err := c.upstream.PullAppFileMeta(vas, req)
	if err != nil {
		return nil, fmt.Errorf("pull file meta failed, err: %s, rid: %s", err.Error(), vas.Rid)
	}
	files := make([]*ConfigItemFile, len(resp.FileMetas))
	for i, meta := range resp.FileMetas {
		files[i] = &ConfigItemFile{
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

	r := &Release{
		ReleaseID: resp.ReleaseId,
		FileItems: files,
		PreHook:   resp.PreHook,
		PostHook:  resp.PostHook,
	}
	return r, nil
}

// GetRelease get release from remote
func (c *client) PullKvs(app string, match []string, opts ...AppOption) (*Release, error) {
	option := &AppOptions{}
	for _, opt := range opts {
		opt(option)
	}
	vas, _ := c.buildVas()
	req := &pbfs.PullKvMetaReq{
		BizId: c.opts.bizID,
		Match: match,
		AppMeta: &pbfs.AppMeta{
			App:    app,
			Labels: c.opts.labels,
			Uid:    c.opts.uid,
		},
	}
	// merge labels, if key conflict, app value will overwrite client value
	req.AppMeta.Labels = util.MergeLabels(c.opts.labels, option.Labels)
	// reset uid
	if option.UID != "" {
		req.AppMeta.Uid = option.UID
	}
	resp, err := c.upstream.PullKvMeta(vas, req)
	if err != nil {
		return nil, err
	}

	kvs := make([]*sfs.KvMetaV1, 0, len(resp.GetKvMetas()))
	for _, v := range resp.GetKvMetas() {
		kvs = append(kvs, &sfs.KvMetaV1{
			Key:          v.GetKey(),
			KvType:       v.KvType,
			Revision:     v.GetRevision(),
			KvAttachment: v.GetKvAttachment(),
		})
	}

	r := &Release{
		ReleaseID: resp.ReleaseId,
		FileItems: []*ConfigItemFile{},
		KvItems:   kvs,
		PreHook:   nil,
		PostHook:  nil,
	}
	return r, nil
}

// Get 读取 Key 的值
func (c *client) Get(app string, key string, opts ...AppOption) (string, error) {
	option := &AppOptions{}
	for _, opt := range opts {
		opt(option)
	}
	vas, _ := c.buildVas()
	req := &pbfs.GetKvValueReq{
		BizId: c.opts.bizID,
		AppMeta: &pbfs.AppMeta{
			App:    app,
			Labels: c.opts.labels,
			Uid:    c.opts.uid,
		},
		Key: key,
	}
	req.AppMeta.Labels = util.MergeLabels(c.opts.labels, option.Labels)
	// reset uid
	if option.UID != "" {
		req.AppMeta.Uid = option.UID
	}
	resp, err := c.upstream.GetKvValue(vas, req)
	if err != nil {
		return "", err
	}

	return resp.Value, nil
}

// ListApps list app from remote, only return have perm by token
func (c *client) ListApps(match []string) ([]*pbfs.App, error) {
	vas, _ := c.buildVas()
	req := &pbfs.ListAppsReq{
		BizId: c.opts.bizID,
		Match: match,
	}
	resp, err := c.upstream.ListApps(vas, req)
	if err != nil {
		return nil, err
	}

	return resp.Apps, nil
}

func (c *client) buildVas() (*kit.Vas, context.CancelFunc) { // nolint
	vas := kit.OutgoingVas(c.pairs)
	ctx, cancel := context.WithCancel(vas.Ctx)
	vas.Ctx = ctx
	return vas, cancel
}
