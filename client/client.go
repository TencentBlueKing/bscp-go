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
	"errors"
	"fmt"
	"time"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/criteria/constant"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	pbbase "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/base"
	pbfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/feed-server"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"github.com/allegro/bigcache/v3"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	// PullKvs pull KV release from remote
	PullKvs(app string, match []string, opts ...AppOption) (*Release, error)
	// Get gets Key Value from remote
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

// ErrNotFoundKvMD5 is err not found kv md5
var ErrNotFoundKvMD5 = errors.New("not found kv md5")

// Client is the bscp client
type client struct {
	pairs    map[string]string
	opts     options
	watcher  *watcher
	upstream upstream.Upstream
}

// New return a bscp client instance
func New(opts ...Option) (Client, error) {
	clientOpt := &options{}
	fp, err := util.GenerateFingerPrint()
	if err != nil {
		return nil, fmt.Errorf("generate instance fingerprint failed, err: %s", err.Error())
	}
	logger.Info("instance fingerprint", slog.String("fingerprint", fp))
	clientOpt.fingerprint = fp
	clientOpt.uid = clientOpt.fingerprint
	for _, opt := range opts {
		if e := opt(clientOpt); e != nil {
			return nil, e
		}
	}
	// prepare pairs
	pairs := make(map[string]string)

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
		opts:     *clientOpt,
		upstream: u,
		pairs:    pairs,
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
	err = downloader.Init(vas, clientOpt.bizID, clientOpt.token, u, pl.RuntimeOption.RepositoryTLS,
		pl.RuntimeOption.EnableAsyncDownload, clientOpt.enableP2PDownload, clientOpt.bkAgentID, clientOpt.clusterID,
		clientOpt.podID, clientOpt.containerName)
	if err != nil {
		return nil, fmt.Errorf("init downloader failed, err: %s", err.Error())
	}

	if err = initFileCache(clientOpt); err != nil {
		return nil, err
	}
	if err = initKvCache(clientOpt); err != nil {
		return nil, err
	}

	watcher, err := newWatcher(u, clientOpt)
	if err != nil {
		return nil, fmt.Errorf("init watcher failed, err: %s", err.Error())
	}
	c.watcher = watcher
	return c, nil
}

// initFileCache init file cache
func initFileCache(opts *options) error {
	if opts.fileCache.Enabled {
		logger.Info("enable file cache")
		if err := cache.Init(opts.fileCache.CacheDir, opts.fileCache.ThresholdGB); err != nil {
			return fmt.Errorf("init file cache failed, err: %s", err.Error())
		}
		go cache.AutoCleanupFileCache(opts.fileCache.CacheDir, DefaultCleanupIntervalSeconds,
			opts.fileCache.ThresholdGB, DefaultCacheRetentionRate)
	}
	return nil
}

// initKvCache init kv cache
func initKvCache(opts *options) error {
	if opts.kvCache.Enabled {
		logger.Info("enable kv cache")
		if err := cache.InitMemCache(opts.kvCache.ThresholdMB); err != nil {
			return fmt.Errorf("init kv cache failed, err: %s", err.Error())
		}

		go func() {
			mc := cache.GetMemCache()
			for {
				hit, miss, kvCnt := mc.Stats().Hits, mc.Stats().Misses, mc.Len()
				var hitRatio float64
				if hit+miss > 0 {
					hitRatio = float64(hit) / float64(hit+miss)
				}
				logger.Debug("kv cache statistics", slog.Int64("hit", hit), slog.Int64("miss", miss),
					slog.String("hit-ratio", fmt.Sprintf("%.3f", hitRatio)), slog.Int("kv-count", kvCnt))
				time.Sleep(time.Second * 15)
			}
		}()
	}
	return nil
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
func (c *client) PullFiles(app string, opts ...AppOption) (*Release, error) { // nolint
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
		Match: option.Match,
	}
	// compatible with the old version of bscp server which can only recognize param req.Key
	if len(option.Match) > 0 {
		req.Key = option.Match[0]
	}
	// merge labels, if key conflict, app value will overwrite client value
	req.AppMeta.Labels = util.MergeLabels(c.opts.labels, option.Labels)
	// reset uid
	if option.UID != "" {
		req.AppMeta.Uid = option.UID
	}
	if req.AppMeta.Uid == "" {
		req.AppMeta.Uid = c.opts.fingerprint
	}

	var err error
	var resp *pbfs.PullAppFileMetaResp
	r := &Release{
		upstream: c.upstream,
		vas:      vas,
		AppMate: &sfs.SideAppMeta{
			App:       app,
			Labels:    req.AppMeta.Labels,
			Uid:       req.AppMeta.Uid,
			Match:     option.Match,
			StartTime: time.Now().UTC(),
		},
	}

	defer func() {
		if err != nil {
			r.AppMate.CursorID = util.GenerateCursorID(c.opts.bizID)
			r.AppMate.ReleaseChangeStatus = sfs.Failed
			r.AppMate.EndTime = time.Now().UTC()
			r.AppMate.TotalSeconds = r.AppMate.EndTime.Sub(r.AppMate.StartTime).Seconds()
			r.AppMate.FailedReason = sfs.AppMetaFailed
			r.AppMate.SpecificFailedReason = sfs.NoDownloadPermission
			r.AppMate.FailedDetailReason = fmt.Sprintf("pull app file meta failed, err: %s", err.Error())
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.PermissionDenied || st.Code() == codes.Unauthenticated {
					r.AppMate.FailedReason = sfs.TokenFailed
					r.AppMate.SpecificFailedReason = sfs.TokenPermissionFailed
				}
				if st.Code() == codes.FailedPrecondition {
					for _, detail := range st.Details() {
						if d, ok := detail.(*pbbase.ErrDetails); ok {
							r.AppMate.FailedReason = sfs.FailedReason(d.PrimaryError)
							r.AppMate.SpecificFailedReason = sfs.SpecificFailedReason(d.SecondaryError)
						}
					}
				}
				r.AppMate.FailedDetailReason = st.Err().Error()
			}
			if err = c.sendClientMessaging(vas, r.AppMate, nil); err != nil {
				logger.Error("description failed to report the client change event",
					slog.String("client_mode", r.ClientMode.String()), slog.Uint64("biz", uint64(r.BizID)),
					slog.String("app", r.AppMate.App), logger.ErrAttr(err))
			}
		}
	}()

	resp, err = c.upstream.PullAppFileMeta(vas, req)
	if err != nil {
		logger.Error("pull file meta failed", logger.ErrAttr(err), slog.String("rid", vas.Rid))
		return nil, err
	}

	files := make([]*ConfigItemFile, len(resp.FileMetas))
	// 计算总文件大小和总文件数
	var totalFileSize uint64
	for i, meta := range resp.FileMetas {
		totalFileSize += meta.CommitSpec.GetContent().ByteSize
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
				ConfigItemRevision:   meta.ConfigItemRevision,
				RepositoryPath:       meta.RepositorySpec.Path,
			},
		}
	}

	r.ReleaseID = resp.ReleaseId
	r.ReleaseName = resp.ReleaseName
	r.FileItems = files
	r.PreHook = resp.PreHook
	r.PostHook = resp.PostHook
	r.AppMate.TargetReleaseID = resp.ReleaseId
	r.AppMate.TotalFileNum = len(files)
	r.AppMate.TotalFileSize = totalFileSize

	return r, nil
}

// PullKvs get release from remote
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
			ContentSpec:  v.GetContentSpec(),
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
// 先从feed-server服务端拉取最新版本元数据，优先从缓存中获取该最新版本value，缓存中没有再调用feed-server获取value并缓存起来
// 在feed-server服务端连接不可用时则降级从缓存中获取（如果有缓存过），此时存在从缓存获取到的value值不是最新发布版本的风险
func (c *client) Get(app string, key string, opts ...AppOption) (string, error) {
	// get kv value from cache
	var val, md5 string
	var err error
	cacheKey := kvCacheKey(c.opts.bizID, app, key)
	if cache.EnableMemCache {
		val, md5, err = c.getKvValueFromCache(app, key, opts...)
		if err == nil {
			return val, nil
		} else if err != bigcache.ErrEntryNotFound {
			logger.Error("get kv value from cache failed", slog.String("key", cacheKey), logger.ErrAttr(err))
		}
	}

	// get kv value from feed-server
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
		st, _ := status.FromError(err)
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Internal:
			logger.Error("feed-server is unavailable", logger.ErrAttr(err))
			// 降级从缓存中获取
			if cache.EnableMemCache {
				v, cErr := cache.GetMemCache().Get(cacheKey)
				if cErr != nil {
					logger.Error("get kv value from cache failed", slog.String("key", cacheKey), logger.ErrAttr(cErr))
					return "", err
				}
				logger.Warn("feed-server is unavailable but get kv value from cache successfully",
					slog.String("key", cacheKey))
				return string(v[32:]), nil
			}
		default:
			return "", err
		}
	}
	val = resp.Value

	// set kv md5 and value for cache
	if cache.EnableMemCache {
		if md5 == "" {
			logger.Error("set kv cache failed", slog.String("key", cacheKey), logger.ErrAttr(ErrNotFoundKvMD5))
		} else {
			if err := cache.GetMemCache().Set(cacheKey, append([]byte(md5), []byte(val)...)); err != nil {
				logger.Error("set kv cache failed", slog.String("key", cacheKey), logger.ErrAttr(err))
			}
		}
	}

	return val, nil
}

// getKvValueWithCache get kv value from the cache
func (c *client) getKvValueFromCache(app string, key string, opts ...AppOption) (string, string, error) {
	release, err := c.PullKvs(app, []string{}, opts...)
	if err != nil {
		return "", "", err
	}

	var md5 string
	for _, k := range release.KvItems {
		if k.Key == key {
			md5 = k.ContentSpec.Md5
			break
		}
	}
	if md5 == "" {
		return "", "", ErrNotFoundKvMD5
	}

	var val []byte
	val, err = cache.GetMemCache().Get(kvCacheKey(c.opts.bizID, app, key))
	if err != nil {
		return "", md5, err
	}
	// 判断是否为最新版本缓存，不是最新则仍从服务端获取value
	if string(val[:32]) != md5 {
		return "", md5, bigcache.ErrEntryNotFound
	}

	return string(val[32:]), md5, nil
}

// kvCacheKey is cache key for kv md5 and value, the cached data's first 32 character is md5, other is value
func kvCacheKey(bizID uint32, app, key string) string {
	return fmt.Sprintf("%d_%s_%s", bizID, app, key)
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

// sendClientMessaging 发送客户端连接信息
func (c *client) sendClientMessaging(vas *kit.Vas, meta *sfs.SideAppMeta, annotations map[string]interface{}) error {
	meta.FailedDetailReason = util.TruncateString(meta.FailedDetailReason, 1024)
	clientInfoPayload := sfs.VersionChangePayload{
		BasicData: &sfs.BasicData{
			BizID:         c.opts.bizID,
			ClientMode:    sfs.Pull,
			ClientType:    sfs.ClientType(version.CLIENTTYPE),
			ClientVersion: version.Version().Version,
			IP:            util.GetClientIP(),
			Annotations:   annotations,
		},
		Application:   meta,
		ResourceUsage: sfs.ResourceUsage{},
	}

	payload, err := clientInfoPayload.Encode()
	if err != nil {
		return err
	}

	_, err = c.upstream.Messaging(vas, clientInfoPayload.MessagingType(), payload)
	if err != nil {
		return err
	}
	return nil
}
