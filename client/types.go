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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/TencentBlueKing/bscp-go/internal/util/process_collect"
	"github.com/TencentBlueKing/bscp-go/pkg/logger"
)

const (
	// updateFileConcurrentLimit is the limit of concurrent for update file.
	updateFileConcurrentLimit = 10
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
	// TextLineBreak text file line break
	TextLineBreak string `json:"textLineBreak"`
	// Permission file permission
	Permission *pbci.FilePermission `json:"permission"`
	// FileMeta data
	FileMeta *sfs.ConfigItemMetaV1 `json:"fileMeta"`
}

// GetContent Get file binary content from cache or download from remote
func (c *ConfigItemFile) GetContent() ([]byte, error) {
	if cache.Enable {
		if hit, bytes := cache.GetCache().GetFileContent(c.FileMeta); hit {
			logger.Debug("get file content from cache success", slog.String("file", filepath.Join(c.Path, c.Name)))
			return bytes, nil
		}
	}
	bytes := make([]byte, c.FileMeta.ContentSpec.ByteSize)

	if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
		c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToBytes, bytes, ""); err != nil {
		logger.Error("download file failed", logger.ErrAttr(err))
		return nil, err
	}
	logger.Debug("get file content by downloading from repo success", slog.String("file", filepath.Join(c.Path, c.Name)))
	return bytes, nil
}

// SaveToFile save file content and write to local file
func (c *ConfigItemFile) SaveToFile(dst string) error {
	// 1. check if cache hit, copy from cache
	if cache.Enable && cache.GetCache().CopyToFile(c.FileMeta, dst) {
		logger.Debug("copy file from cache success", slog.String("dst", dst))
	} else {
		// 2. if cache not hit, download file from remote
		if err := downloader.GetDownloader().Download(c.FileMeta.PbFileMeta(), c.FileMeta.RepositoryPath,
			c.FileMeta.ContentSpec.ByteSize, downloader.DownloadToFile, nil, dst); err != nil {
			logger.Error("download file failed", logger.ErrAttr(err))
			return err
		}
	}
	// 3. check whether need to convert line break
	if c.FileMeta.ConfigItemSpec.FileType == "text" && c.TextLineBreak != "" {
		if err := util.ConvertTextLineBreak(dst, c.TextLineBreak); err != nil {
			logger.Error("convert text file line break failed", slog.String("file", dst), logger.ErrAttr(err))
			return err
		}
	}

	return nil
}

// Callback watch callback
type Callback func(release *Release) error

// Function 定义类型
type Function func() error

// compareRelease 对比当前服务版本、配置匹配规则
// 返回值: 是否跳过本次版本变更事件（本地已有和事件的版本、配置匹配规则一致）, 错误信息
func (r *Release) compareRelease() (bool, error) {
	lastMetadata, exist, err := eventmeta.GetLatestMetadataFromFile(r.AppDir)
	if err != nil {
		logger.Error("get metadata file failed", logger.ErrAttr(err))
		return false, err
	}
	// 如果 metadata 文件不存在，说明没有执行过 pull 操作
	if !exist {
		logger.Warn("can not find metadata file, maybe you should exec pull command first")
		return false, nil
	}
	if lastMetadata.ReleaseID == r.ReleaseID && util.StrSlicesEqual(lastMetadata.ConfigMatches, r.AppMate.Match) {
		r.AppMate.CurrentReleaseID = r.ReleaseID
		logger.Info("current release is consistent with the received release, skip", slog.Any("releaseID", r.ReleaseID))
		return true, nil
	}
	r.AppMate.CurrentReleaseID = lastMetadata.ReleaseID
	return false, nil
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
		logger.Error("execute pre hook", logger.ErrAttr(err))
		// 断言错误
		var smallErr sfs.SecondaryError
		if errors.As(err, &smallErr) {
			return sfs.WrapPrimaryError(sfs.PreHookFailed, smallErr)
		}
		// 前置未知错误
		return sfs.WrapPrimaryError(sfs.PreHookFailed, sfs.SecondaryError{
			SpecificFailedReason: sfs.UnknownSpecificFailed,
			Err:                  err,
		})
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
		logger.Error("execute post hook", logger.ErrAttr(err))
		// 断言错误
		var smallErr sfs.SecondaryError
		if errors.As(err, &smallErr) {
			return sfs.WrapPrimaryError(sfs.PostHookFailed, smallErr)
		}
		// 后置未知错误
		return sfs.WrapPrimaryError(sfs.PostHookFailed, sfs.SecondaryError{
			SpecificFailedReason: sfs.UnknownSpecificFailed,
			Err:                  err,
		})
	}
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
		filesDir := filepath.Join(r.AppDir, "files")
		if err := updateFiles(filesDir, r.FileItems, &r.AppMate.DownloadFileNum, &r.AppMate.DownloadFileSize,
			r.SemaphoreCh); err != nil {
			logger.Error("update file failed", logger.ErrAttr(err))
			return err
		}
		if r.ClientMode == sfs.Pull {
			return nil
		}
		if err := clearOldFiles(filesDir, r.FileItems); err != nil {
			logger.Error("clear old files failed", logger.ErrAttr(err))
			// 断言错误，可能是遍历文件错误
			var smallErr sfs.SecondaryError
			if errors.As(err, &smallErr) {
				return sfs.WrapPrimaryError(sfs.DeleteOldFilesFailed, smallErr)
			}
			// 删除错误
			return sfs.WrapPrimaryError(sfs.DeleteOldFilesFailed,
				sfs.SecondaryError{
					SpecificFailedReason: sfs.DeleteFolderFailed,
					Err:                  err,
				})
		}
		return nil
	}
}

// UpdateMetadata 4.更新meatdata数据方法
func (r *Release) UpdateMetadata() Function {
	return func() error {
		match := r.AppMate.Match
		if match == nil {
			match = []string{}
		}
		metadata := &eventmeta.EventMeta{
			ReleaseID:     r.ReleaseID,
			Status:        eventmeta.EventStatusSuccess,
			ConfigMatches: match,
			EventTime:     time.Now().Format(time.RFC3339),
		}
		err := eventmeta.AppendMetadataToFile(r.AppDir, metadata)
		if err != nil {
			logger.Error("append metadata to file failed", logger.ErrAttr(err))
			return err
		}
		return nil
	}
}

// checkFileExists checks the file exists and the SHA256 is match.
func checkFileExists(absPath string, ci *sfs.ConfigItemMetaV1) (bool, error) {
	filePath := filepath.Join(absPath, ci.ConfigItemSpec.Name)
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
		logger.Debug("configuration item's SHA256 is not match, need to update",
			slog.String("localHash", sha), slog.String("remoteHash", ci.ContentSpec.Signature))
		return false, nil
	}

	return true, nil
}

// clearOldFiles 删除旧文件
func clearOldFiles(dir string, files []*ConfigItemFile) error {
	if _, err := os.Stat(dir); err != nil {
		// 根目录dir不存在，则之前没有拉取到任何匹配的文件，不用清理，直接退出
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	err := filepath.Walk(dir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return sfs.WrapPrimaryError(sfs.DeleteOldFilesFailed,
				sfs.SecondaryError{SpecificFailedReason: sfs.TraverseFolderFailed,
					Err: err})
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
			logger.Info("delete folder success", slog.String("folder", filePath))
			return filepath.SkipDir
		}

		for _, file := range files {
			absFile := filepath.Join(dir, file.Path, file.Name)
			if absFile == filePath {
				return nil
			}
		}
		if err := os.Remove(filePath); err != nil {
			return err
		}
		logger.Info("delete file success", slog.String("file", filePath))
		return nil
	})

	return err
}

// Execute 统一执行入口
func (r *Release) Execute(steps ...Function) error {
	var err error
	// 填充appMate数据
	r.AppMate.CursorID = r.CursorID
	r.AppMate.StartTime = time.Now().UTC()
	r.AppMate.ReleaseChangeStatus = sfs.Processing
	// 初始化基础数据
	bd := r.handleBasicData(r.ClientMode, map[string]interface{}{})

	// 发送变更事件
	defer func() {
		r.AppMate.EndTime = time.Now().UTC()
		r.AppMate.TotalSeconds = r.AppMate.EndTime.Sub(r.AppMate.StartTime).Seconds()
		r.AppMate.ReleaseChangeStatus = sfs.Success
		if err != nil {
			// 默认为未知错误
			r.AppMate.ReleaseChangeStatus = sfs.Failed
			r.AppMate.FailedReason = sfs.UnknownFailed
			r.AppMate.SpecificFailedReason = sfs.UnknownSpecificFailed
			r.AppMate.FailedDetailReason = err.Error()
			var e sfs.PrimaryError
			if errors.As(err, &e) {
				r.AppMate.FailedReason = e.FailedReason
				r.AppMate.SpecificFailedReason = e.SpecificFailedReason
				r.AppMate.FailedDetailReason = e.Err.Error()
			}
		}

		if err = r.sendVersionChangeMessaging(bd); err != nil {
			logger.Error("description failed to report the client change event",
				slog.String("client_mode", r.ClientMode.String()), slog.Uint64("biz", uint64(r.BizID)),
				slog.String("app", r.AppMate.App), logger.ErrAttr(err))
		}

	}()

	// 一定要在该位置
	// 不然会导致current_release_id是0的问题
	var skip bool
	if r.ClientMode == sfs.Watch {
		skip, err = r.compareRelease()
		if err != nil {
			return err
		}
	}

	// 发送拉取前事件
	if err = r.sendVersionChangeMessaging(bd); err != nil {
		logger.Error("failed to send the pull status event", slog.Uint64("biz", uint64(r.BizID)),
			slog.String("app", r.AppMate.App), logger.ErrAttr(err))
	}

	// 发送心跳数据
	if r.ClientMode == sfs.Pull {
		r.loopHeartbeat(bd)
	}

	if skip {
		return nil
	}

	for _, step := range steps {
		if err = step(); err != nil {
			return err
		}
	}

	return nil
}

// sendVersionChangeMessaging 发送客户端版本变更信息
func (r *Release) sendVersionChangeMessaging(bd *sfs.BasicData) error {
	r.AppMate.FailedDetailReason = util.TruncateString(r.AppMate.FailedDetailReason, 1024)
	pullPayload := sfs.VersionChangePayload{
		BasicData:     bd,
		Application:   r.AppMate,
		ResourceUsage: getResourceUsage(),
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

// getResource 获取cpu和内存使用信息
func getResourceUsage() sfs.ResourceUsage {
	cpuUsage, cpuMaxUsage, cpuMinUsage, cpuAvgUsage := process_collect.GetCpuUsage()
	memoryUsage, memoryMaxUsage, memoryMinUsage, memoryAvgUsage := process_collect.GetMemUsage()
	return sfs.ResourceUsage{
		MemoryUsage:    memoryUsage,
		MemoryMaxUsage: memoryMaxUsage,
		MemoryMinUsage: memoryMinUsage,
		MemoryAvgUsage: memoryAvgUsage,
		CpuUsage:       cpuUsage,
		CpuMaxUsage:    cpuMaxUsage,
		CpuMinUsage:    cpuMinUsage,
		CpuAvgUsage:    cpuAvgUsage,
	}

}

// pull时定时上报心跳
func (r *Release) loopHeartbeat(bd *sfs.BasicData) {
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
					BasicData:     *bd,
					Applications:  apps,
					ResourceUsage: getResourceUsage(),
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
	start := time.Now()
	// Initialize the successDownloads and successFileSize to zero at the beginning of the function.
	atomic.StoreInt32(successDownloads, 0)
	atomic.StoreUint64(successFileSize, 0)
	var success, failed, skip int32
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(updateFileConcurrentLimit)
	for _, f := range files {
		file := f
		g.Go(func() error {
			// 1. prapare file path
			fileDir := filepath.Join(filesDir, file.Path)
			filePath := filepath.Join(fileDir, file.Name)
			err := os.MkdirAll(fileDir, os.ModePerm)
			if err != nil {
				atomic.AddInt32(&failed, 1)
				return sfs.WrapPrimaryError(sfs.DownloadFailed,
					sfs.SecondaryError{SpecificFailedReason: sfs.NewFolderFailed,
						Err: fmt.Errorf("create dir %s failed, err: %s", fileDir, err.Error())})
			}
			// 2. check and download file
			exists, err := checkFileExists(fileDir, file.FileMeta)
			if err != nil {
				atomic.AddInt32(&failed, 1)
				return sfs.WrapPrimaryError(sfs.DownloadFailed,
					sfs.SecondaryError{SpecificFailedReason: sfs.CheckFileExistsFailed,
						Err: fmt.Errorf("check file exists failed, err: %s", err.Error())})
			}
			if !exists {
				err := file.SaveToFile(filePath)
				if err != nil {
					atomic.AddInt32(&failed, 1)
					return err
				}
				atomic.AddInt32(&success, 1)
				logger.Info("update file success", slog.String("file", filePath))
			} else {
				atomic.AddInt32(&skip, 1)
				logger.Debug("file is already exists and has not been modified, skip download",
					slog.String("file", filePath))
			}
			// 3. set file permission
			if runtime.GOOS != "windows" {
				if err := util.SetFilePermission(filePath, file.FileMeta.ConfigItemSpec.Permission); err != nil {
					logger.Warn("set file permission failed", slog.String("file", filePath), logger.ErrAttr(err))
				}
			}
			atomic.AddInt32(successDownloads, 1)
			atomic.AddUint64(successFileSize, file.FileMeta.ContentSpec.ByteSize)
			semaphoreCh <- struct{}{}
			return nil
		})
	}
	err := g.Wait()

	logger.Info("update files done", slog.Int("success", int(success)), slog.Int("skip", int(skip)),
		slog.Int("failed", int(failed)), slog.Int("total", len(files)),
		slog.String("duration", time.Since(start).String()))

	if err != nil {
		var e sfs.PrimaryError
		if errors.As(err, &e) {
			return err
		}
		return sfs.WrapPrimaryError(sfs.DownloadFailed,
			sfs.SecondaryError{SpecificFailedReason: sfs.UnknownSpecificFailed,
				Err: err})
	}

	return nil
}
