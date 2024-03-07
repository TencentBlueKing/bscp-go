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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/kit"
	pbci "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/config-item"
	pbhook "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/protocol/core/hook"
	sfs "github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/sf-share"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/tools"
	"github.com/TencentBlueKing/bk-bcs/bcs-services/bcs-bscp/pkg/version"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"

	"github.com/TencentBlueKing/bscp-go/internal/cache"
	"github.com/TencentBlueKing/bscp-go/internal/downloader"
	"github.com/TencentBlueKing/bscp-go/internal/upstream"
	"github.com/TencentBlueKing/bscp-go/internal/util"
	"github.com/TencentBlueKing/bscp-go/internal/util/eventmeta"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	// updateFileConcurrentLimit is the limit of concurrent for update file.
	updateFileConcurrentLimit = 5
)

// Release bscp 服务版本
type Release struct {
	ReleaseID   uint32            `json:"release_id"`
	ReleaseName string            `json:"release_name"`
	FileItems   []*ConfigItemFile `json:"files"`
	KvItems     []*sfs.KvMetaV1   `json:"kvs"`
	PreHook     *pbhook.HookSpec  `json:"pre_hook"`
	PostHook    *pbhook.HookSpec  `json:"post_hook"`
	CursorID    string            `json:"cursor_id"`
	SemaphoreCh chan struct{}
	upstream    upstream.Upstream
	vas         *kit.Vas
	AppDir      string
	TempDir     string
	BizID       uint32
	ClientMode  sfs.ClientMode
	AppMate     *sfs.SideAppMeta
}

// ConfigItemFile defines config item file
type ConfigItemFile struct {
	// Config file name
	Name string `json:"name"`
	// Path of config file
	Path string `json:"path"`
	// Permission file permission
	Permission *pbci.FilePermission `json:"permission"`
	// FileMeta data
	FileMeta *sfs.ConfigItemMetaV1 `json:"fileMeta"`
}

// GetContent Get file binary content from cache or download from remote
func (c *ConfigItemFile) GetContent() ([]byte, error) {
	if cache.Enable {
		if hit, bytes := cache.GetCache().GetFileContent(c.FileMeta); hit {
			logger.Info("get file content from cache success", slog.String("file", path.Join(c.Path, c.Name)))
			return bytes, nil
		}
	}
	bytes := make([]byte, c.FileMeta.ContentSpec.ByteSize)

	if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
		c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToBytes, bytes, ""); err != nil {
		return nil, fmt.Errorf("download file failed, err: %s", err.Error())
	}
	logger.Info("get file content by downloading from repo success", slog.String("file", path.Join(c.Path, c.Name)))
	return bytes, nil
}

// SaveToFile save file content and write to local file
func (c *ConfigItemFile) SaveToFile(dst string) error {
	// 1. check if cache hit, copy from cache
	if cache.Enable && cache.GetCache().CopyToFile(c.FileMeta, dst) {
		logger.Info("copy file from cache success", slog.String("dst", dst))
	} else {
		// 2. if cache not hit, download file from remote
		if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
			c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToFile, nil, dst); err != nil {
			return fmt.Errorf("download file failed, err %s", err.Error())
		}
	}

	return nil
}

// Callback watch callback
type Callback func(release *Release) error

// Function 定义类型
type Function func() error

// CompareFile 对比文件信息
func (r *Release) compareFile() error {
	if r.ClientMode == sfs.Pull {
		return nil
	}
	lastMetadata, err := eventmeta.GetLatestMetadataFromFile(r.AppDir)
	if err != nil {
		r.AppMate.FailedReason = sfs.DownloadFailed
		logger.Warn("get latest release metadata failed, maybe you should exec pull command first", logger.ErrAttr(err))
		return err
	} else if lastMetadata.ReleaseID == r.ReleaseID {
		r.AppMate.CurrentReleaseID = r.ReleaseID
		r.AppMate.FailedReason = sfs.SkipFailed
		logger.Info("current release is consistent with the received release, skip", slog.Any("releaseID", r.ReleaseID))
		return nil
	}
	r.AppMate.CurrentReleaseID = lastMetadata.ReleaseID
	return nil
}

// ExecuteHook 1.执行脚本方法
// 根据策略执行不同脚本
func (r *Release) ExecuteHook(hook ScriptStrategy) Function {
	return func() error {
		return hook.executeScript(r)
	}
}

// UpdateFiles 2.下载文件方法
func (r *Release) UpdateFiles() Function {
	return func() error {
		filesDir := path.Join(r.AppDir, "files")
		if err := updateFiles(filesDir, r.FileItems, &r.AppMate.DownloadFileNum,
			&r.AppMate.DownloadFileSize, r.SemaphoreCh); err != nil {
			r.AppMate.FailedReason = sfs.DownloadFailed
			logger.Error("update files", logger.ErrAttr(err))
			return err
		}
		if r.ClientMode == sfs.Pull {
			return nil
		}
		if err := clearOldFiles(filesDir, r.FileItems); err != nil {
			r.AppMate.FailedReason = sfs.ClearOldFilesFailed
			logger.Error("clear old files failed", logger.ErrAttr(err))
			return err
		}
		return nil
	}
}

// UpdateMetadata 4.更新meatdata数据方法
func (r *Release) UpdateMetadata() Function {
	return func() error {
		metadata := &eventmeta.EventMeta{
			ReleaseID: r.ReleaseID,
			Status:    eventmeta.EventStatusSuccess,
			EventTime: time.Now().Format(time.RFC3339),
		}
		err := eventmeta.AppendMetadataToFile(r.AppDir, metadata)
		if err != nil {
			r.AppMate.FailedReason = sfs.UpdateMetadataFailed
			logger.Error("append metadata to file failed", logger.ErrAttr(err))
			return err
		}
		return nil
	}
}

// ScriptStrategy 定义脚本接口
type ScriptStrategy interface {
	executeScript(r *Release) error
}

// PreScriptStrategy 前置脚本
type PreScriptStrategy struct{}

// executeScript 执行前置脚本
func (p *PreScriptStrategy) executeScript(r *Release) error {
	if r.PreHook == nil {
		return nil
	}
	err := util.ExecuteHook(r.PreHook, table.PreHook, r.TempDir, r.BizID, r.AppMate.App, r.ReleaseName)
	if err != nil {
		r.AppMate.FailedReason = sfs.PreHookFailed
		logger.Error("execute pre hook", logger.ErrAttr(err))
		return err
	}
	return nil
}

// PostScriptStrategy 后置脚本
type PostScriptStrategy struct{}

// executeScript 执行后置脚本
func (p *PostScriptStrategy) executeScript(r *Release) error {
	if r.PostHook == nil {
		return nil
	}
	err := util.ExecuteHook(r.PostHook, table.PostHook, r.TempDir, r.BizID, r.AppMate.App, r.ReleaseName)
	if err != nil {
		r.AppMate.FailedReason = sfs.PostHookFailed
		logger.Error("execute post hook", logger.ErrAttr(err))
		return err
	}
	return nil
}

// checkFileExists checks the file exists and the SHA256 is match.
func checkFileExists(absPath string, ci *sfs.ConfigItemMetaV1) (bool, error) {
	filePath := path.Join(absPath, ci.ConfigItemSpec.Name)
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// content is not exist
			return false, nil
		}

		return false, err
	}

	sha, err := tools.FileSHA256(filePath)
	if err != nil {
		return false, fmt.Errorf("check configuration item's SHA256 failed, err: %s", err.Error())
	}

	if sha != ci.ContentSpec.Signature {
		logger.Info("configuration item's SHA256 is not match, need to update",
			slog.String("localHash", sha), slog.String("remoteHash", ci.ContentSpec.Signature))
		return false, nil
	}

	return true, nil
}

// clearOldFiles 删除旧文件
func clearOldFiles(dir string, files []*ConfigItemFile) error {
	err := filepath.Walk(dir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			for _, file := range files {
				absFileDir := filepath.Join(dir, file.Path)
				if strings.HasPrefix(absFileDir, filePath) {
					return nil
				}
			}
			if err := os.RemoveAll(filePath); err != nil {
				return err
			}
			return filepath.SkipDir
		}

		for _, file := range files {
			absFile := filepath.Join(dir, file.Path, file.Name)
			if absFile == filePath {
				return nil
			}
		}
		return os.Remove(filePath)
	})

	return err
}

// Execute 统一执行入口
func (r *Release) Execute(steps ...Function) error {
	// 填充appMate数据
	r.AppMate.CursorID = r.CursorID
	r.AppMate.StartTime = time.Now()
	r.AppMate.ReleaseChangeStatus = sfs.Processing
	// 初始化基础数据
	bd := r.handleBasicData(r.ClientMode, map[string]interface{}{})
	// 获取cpu和内存相关信息
	resource := getResource()
	// 发送变更事件
	defer func() {
		r.AppMate.EndTime = time.Now()
		r.AppMate.TotalSeconds = r.AppMate.EndTime.Sub(r.AppMate.StartTime).Seconds()
		r.AppMate.ReleaseChangeStatus = sfs.Success
		if r.AppMate.FailedReason != 0 && r.AppMate.FailedReason != 4 {
			r.AppMate.ReleaseChangeStatus = sfs.Failed
		}
		err := r.sendVersionChangeMessaging(bd, resource)
		if err != nil {
			logger.Error("description failed to report the client change event, client_mode: %s, biz: %d,app: %s, err: %s",
				r.ClientMode.String(), r.BizID, r.AppMate.App, err.Error())
		}
	}()

	// 一定要在该位置
	// 不然会导致current_release_id是0的问题
	if err := r.compareFile(); err != nil {
		r.AppMate.FailedDetailReason = err.Error()
		return err
	}
	// 发送拉取前事件
	err := r.sendVersionChangeMessaging(bd, resource)
	if err != nil {
		logger.Error("failed to send the pull status event. biz: %d,app: %s, err: %s",
			r.BizID, r.AppMate.App, err.Error())
	}

	// 发送心跳数据
	if r.ClientMode == sfs.Pull {
		r.loopHeartbeat(bd, resource)
	}

	for _, step := range steps {
		err := step()
		if err != nil {
			r.AppMate.FailedDetailReason = err.Error()
			return err
		}
	}

	return nil
}

// sendVersionChangeMessaging 发送客户端版本变更信息
func (r *Release) sendVersionChangeMessaging(bd *sfs.BasicData, usage resource) error {
	pullPayload := sfs.VersionChangePayload{
		BasicData:   bd,
		Application: r.AppMate,
		ResourceUsage: sfs.ResourceUsage{
			CpuUsage:       usage.currentCPUUsage,
			CpuMaxUsage:    usage.maxCPUUsage,
			MemoryUsage:    usage.currentMemoryUsage,
			MemoryMaxUsage: usage.maxMemoryUsage,
		},
	}

	encode, err := pullPayload.Encode()
	if err != nil {
		return err
	}
	_, err = r.upstream.Messaging(r.vas, pullPayload.MessagingType(), encode)
	if err != nil {
		return err
	}
	return nil
}

// handleBasicData 处理基础数据
func (r *Release) handleBasicData(mode sfs.ClientMode, annotations map[string]interface{}) *sfs.BasicData {
	return &sfs.BasicData{
		BizID:         r.BizID,
		ClientMode:    mode,
		ClientVersion: version.Version().Version,
		ClientType:    sfs.ClientType(version.CLIENTTYPE),
		IP:            util.GetClientIP(),
		Annotations:   annotations,
	}
}

type resource struct {
	maxMemoryUsage, currentMemoryUsage uint64
	maxCPUUsage, currentCPUUsage       float64
}

// getResource 获取cpu和内存使用信息
func getResource() resource {
	currentCPUUsage, maxCPUUsage := util.GetCpuUsage()
	currentMemoryUsage, maxMemoryUsage := util.GetMemUsage()
	return resource{
		maxCPUUsage:        maxCPUUsage,
		currentCPUUsage:    currentCPUUsage,
		maxMemoryUsage:     maxMemoryUsage,
		currentMemoryUsage: currentMemoryUsage,
	}

}

// pull时定时上报心跳
func (r *Release) loopHeartbeat(bd *sfs.BasicData, usage resource) {
	go func() {
		tick := time.NewTicker(defaultHeartbeatInterval)
		defer tick.Stop()
		for {
			select {
			case <-r.vas.Ctx.Done():
				logger.Info("stream heartbeat stoped because of ctx done")
				return
			case <-tick.C:
				apps := make([]sfs.SideAppMeta, 0)
				apps = append(apps, *r.AppMate)
				heartbeatPayload := sfs.HeartbeatPayload{
					BasicData:    *bd,
					Applications: apps,
				}
				heartbeatPayload.ResourceUsage = sfs.ResourceUsage{
					CpuMaxUsage:    usage.maxCPUUsage,
					CpuUsage:       usage.currentCPUUsage,
					MemoryMaxUsage: usage.maxMemoryUsage,
					MemoryUsage:    usage.currentMemoryUsage,
				}
				payload, err := heartbeatPayload.Encode()
				if err != nil {
					logger.Error("stream start loop heartbeat failed by encode heartbeat payload", logger.ErrAttr(err))
					return
				}
				if err := r.heartbeatOnce(heartbeatPayload.MessagingType(), payload); err != nil {
					logger.Warn("stream heartbeat failed, notify reconnect upstream",
						logger.ErrAttr(err), slog.String("rid", r.vas.Rid))
					return
				}
				logger.Debug("stream heartbeat successfully", slog.String("rid", r.vas.Rid))
			}
		}
	}()
}

// heartbeatOnce send heartbeat to upstream server, if failed maxHeartbeatRetryCount count, return error.
func (r *Release) heartbeatOnce(msgType sfs.MessagingType, payload []byte) error {
	retry := tools.NewRetryPolicy(maxHeartbeatRetryCount, [2]uint{1000, 3000})

	var lastErr error
	for {
		select {
		case <-r.vas.Ctx.Done():
			return nil
		default:
		}

		if retry.RetryCount() == maxHeartbeatRetryCount {
			return lastErr
		}

		if err := r.sendHeartbeatMessaging(r.vas, msgType, payload); err != nil {
			logger.Error("send heartbeat message failed",
				slog.Any("retry_count", retry.RetryCount()), logger.ErrAttr(err), slog.String("rid", r.vas.Rid))
			lastErr = err
			retry.Sleep()
			continue
		}

		return nil
	}
}

// sendHeartbeatMessaging send heartbeat message to upstream server.
func (r *Release) sendHeartbeatMessaging(vas *kit.Vas, msgType sfs.MessagingType, payload []byte) error {
	timeoutVas, cancel := vas.WithTimeout(defaultHeartbeatTimeout)
	defer cancel()

	if _, err := r.upstream.Messaging(timeoutVas, msgType, payload); err != nil {
		return err
	}

	return nil
}

// updateFiles updates the files to the target directory.
func updateFiles(filesDir string, files []*ConfigItemFile, successDownloads *int32, successFileSize *uint64,
	semaphoreCh chan struct{}) error {
	// var successDownloads int32
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(updateFileConcurrentLimit)
	for _, f := range files {
		file := f
		g.Go(func() error {
			// 1. prapare file path
			fileDir := path.Join(filesDir, file.Path)
			filePath := path.Join(fileDir, file.Name)
			err := os.MkdirAll(fileDir, os.ModePerm)
			if err != nil {
				return fmt.Errorf("create dir %s failed, err: %s", fileDir, err.Error())
			}
			// 2. check and download file
			exists, err := checkFileExists(fileDir, file.FileMeta)
			if err != nil {
				return fmt.Errorf("check file exists failed, err: %s", err.Error())
			}
			if !exists {
				err := downloader.GetDownloader().Download(file.FileMeta.PbFileMeta(), file.FileMeta.RepositoryPath,
					file.FileMeta.ContentSpec.ByteSize, downloader.DownloadToFile, nil, filePath)
				if err != nil {
					return fmt.Errorf("download file failed, err: %s", err.Error())
				}
			} else {
				logger.Info("file is already exists and has not been modified, skip download", slog.String("file", filePath))
			}
			// 3. set file permission
			if err := util.SetFilePermission(filePath, file.FileMeta.ConfigItemSpec.Permission); err != nil {
				logger.Warn("set file permission failed", slog.String("file", filePath), logger.ErrAttr(err))
			}
			atomic.AddInt32(successDownloads, 1)
			atomic.AddUint64(successFileSize, file.FileMeta.ContentSpec.ByteSize)
			semaphoreCh <- struct{}{}
			return nil
		})
	}
	return g.Wait()
}
