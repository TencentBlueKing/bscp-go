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
	"context"
	"fmt"
	"time"

	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
	pbbase "github.com/TencentBlueKing/bk-bscp/pkg/protocol/core/base"
	pbfs "github.com/TencentBlueKing/bk-bscp/pkg/protocol/feed-server"
	sfs "github.com/TencentBlueKing/bk-bscp/pkg/sf-share"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

var (
	// DefaultDialTimeoutMS is the default dial timeout in milliseconds for upstream client.
	DefaultDialTimeoutMS int64 = 2000
)

// Upstream implement all the client which is used to connect with upstream
// servers.
// Note: if the client is reconnecting to the upstream servers, it will block
// all the requests with a timeout so that these requests can use the new connection
// to connect with the upstream server.
type Upstream interface {
	ReconnectUpstreamServer() error
	Handshake(vas *kit.Vas, msg *pbfs.HandshakeMessage) (*pbfs.HandshakeResp, error)
	Watch(vas *kit.Vas, payload []byte) (pbfs.Upstream_WatchClient, error)
	Messaging(vas *kit.Vas, typ sfs.MessagingType, payload []byte) (*pbfs.MessagingResp, error)
	EnableBounce(bounceIntervalHour uint)
	PullAppFileMeta(vas *kit.Vas, req *pbfs.PullAppFileMetaReq) (*pbfs.PullAppFileMetaResp, error)
	PullKvMeta(vas *kit.Vas, req *pbfs.PullKvMetaReq) (*pbfs.PullKvMetaResp, error)
	GetKvValue(vas *kit.Vas, req *pbfs.GetKvValueReq) (*pbfs.GetKvValueResp, error)
	GetDownloadURL(vas *kit.Vas, req *pbfs.GetDownloadURLReq) (*pbfs.GetDownloadURLResp, error)
	Version() *pbbase.Versioning
	ListApps(vas *kit.Vas, req *pbfs.ListAppsReq) (*pbfs.ListAppsResp, error)
	AsyncDownload(vas *kit.Vas, req *pbfs.AsyncDownloadReq) (*pbfs.AsyncDownloadResp, error)
	AsyncDownloadStatus(vas *kit.Vas, req *pbfs.AsyncDownloadStatusReq) (*pbfs.AsyncDownloadStatusResp, error)
	GetSingleFileContent(vas *kit.Vas, req *pbfs.GetSingleFileContentReq) (pbfs.Upstream_GetSingleFileContentClient, error)
}

// New create a rolling client instance.
func New(opts ...Option) (Upstream, error) {

	option := &Options{}
	for _, opt := range opts {
		opt(option)
	}
	if option.DialTimeoutMS <= 0 {
		option.DialTimeoutMS = DefaultDialTimeoutMS
	}
	lb, err := newBalancer(option.FeedAddrs)
	if err != nil {
		return nil, err
	}

	dialOpts := make([]grpc.DialOption, 0)
	// blocks until the connection is established.
	dialOpts = append(dialOpts, grpc.WithBlock()) // nolint:staticcheck
	dialOpts = append(dialOpts, grpc.WithUserAgent("bscp-sdk-golang"))
	// dial without ssl
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	uc := &upstreamClient{
		options: option,
		sidecarVer: &pbbase.Versioning{
			Major: sfs.CurrentAPIVersion.Major,
			Minor: sfs.CurrentAPIVersion.Minor,
			Patch: sfs.CurrentAPIVersion.Patch,
		},
		dialOpts: dialOpts,
		lb:       lb,
		wait:     initBlocker(),
	}

	uc.bounce = initBounce(uc.ReconnectUpstreamServer)

	if err := uc.dial(); err != nil {
		return nil, err
	}

	go uc.waitForStateChange()

	return uc, nil
}

// upstreamClient is an implementation of the upstream server's client, it sends to and receive messages from
// the upstream feed server.
// Note:
// 1. it also hijacked the connections to upstream server so that it can
// do reconnection, bounce work and so on.
// 2. it blocks the request until the connections to the upstream server go back to normal when the connection
// is unavailable.
type upstreamClient struct {
	options    *Options
	sidecarVer *pbbase.Versioning
	dialOpts   []grpc.DialOption
	// cancelCtx cancel ctx is used to cancel the upstream connection.
	cancelCtx context.CancelFunc
	lb        *balancer
	bounce    *bounce

	wait   *blocker
	conn   *grpc.ClientConn
	client pbfs.UpstreamClient
}

// dial blocks until the connection is established.
func (uc *upstreamClient) dial() error {

	if uc.conn != nil {
		if err := uc.conn.Close(); err != nil {
			logger.Error("close the previous connection failed", logger.ErrAttr(err))
			// do not return here, the new connection will be established.
		}
	}

	timeout := uc.options.DialTimeoutMS

	ctx, cancel := context.WithTimeout(context.TODO(), time.Duration(timeout)*time.Millisecond)
	endpoint := uc.lb.PickOne()
	conn, err := grpc.DialContext(ctx, endpoint, uc.dialOpts...) // nolint:staticcheck
	if err != nil {
		cancel()
		uc.cancelCtx = nil
		return fmt.Errorf("dial upstream grpc server failed, err: %s", err.Error())
	}

	logger.Info("dial upstream server success", slog.String("upstream", endpoint))

	uc.cancelCtx = cancel
	uc.conn = conn
	uc.client = pbfs.NewUpstreamClient(conn)

	return nil
}

// Version returns the version of the sdk.
func (uc *upstreamClient) Version() *pbbase.Versioning {
	return uc.sidecarVer
}

// ReconnectUpstreamServer blocks until the new connection is established with dial again.
func (uc *upstreamClient) ReconnectUpstreamServer() error {
	if !uc.wait.TryBlock() {
		logger.Warn("received reconnect to upstream server request, but another reconnect is processing, ignore this")
		return nil
	}
	// got the block lock for now.

	defer uc.wait.Unblock()
	if err := uc.dial(); err != nil {
		return fmt.Errorf("reconnect upstream server failed because of %s", err.Error())
	}

	return nil
}

// EnableBounce set conn reconnect interval, and start loop wait connect bounce. call multiple times,
// you need to wait for the last bounce interval to arrive, the bounce interval of set this time
// will take effect.
func (uc *upstreamClient) EnableBounce(bounceIntervalHour uint) {
	uc.bounce.updateInterval(bounceIntervalHour)

	if !uc.bounce.state() {
		go uc.bounce.enableBounce()
	}
}

// waitForStateChange use the connection state to determine what to do next.
func (uc *upstreamClient) waitForStateChange() {
	for {
		//nolint:staticcheck
		if uc.conn.WaitForStateChange(context.TODO(), connectivity.Ready) {
			// TODO: loop and wait and then determine whether we need to create a new connection
		}
	}
}
