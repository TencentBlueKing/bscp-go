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

// Package logger defines the logger interface.
package logger

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"golang.org/x/exp/slog"
)

var defaultLogger atomic.Value

func init() {
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelInfo,
		ReplaceAttr: ReplaceSourceAttr,
	})
	logger := slog.New(&handler{
		TextHandler: textHandler,
	})
	defaultLogger.Store(logger)
}

type handler struct {
	*slog.TextHandler
}

// Handle 自定义Hanlde, 格式化source 为 dir/path:line, banner 处理
func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	rr := &r

	// pid := os.Getpid()
	// rr.Add(slog.Int("pid", pid))

	var pcs [1]uintptr
	runtime.Callers(5, pcs[:]) // skip [Callers, Infof]
	rr.PC = pcs[0]

	return h.TextHandler.Handle(ctx, *rr)
}

func getLogger() *slog.Logger {
	return defaultLogger.Load().(*slog.Logger)
}

// SetHandler set logger
func SetHandler(handler slog.Handler) {
	defaultLogger.Store(slog.New(handler))
}

// SetLevel init default logger with level
func SetLevel(level slog.Leveler) {
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   true,
		Level:       level,
		ReplaceAttr: ReplaceSourceAttr,
	})
	logger := slog.New(&handler{
		TextHandler: textHandler,
	})
	defaultLogger.Store(logger)
}

// GetLevelByName human readable logger level
func GetLevelByName(name string) slog.Leveler {
	switch name {
	case "error":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "info":
		return slog.LevelInfo
	case "debug":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// Debug logs at LevelDebug.
func Debug(msg string, args ...any) {
	getLogger().Debug(msg, args...)
}

// DebugContext logs at LevelDebug with the given context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	getLogger().DebugContext(ctx, msg, args...)
}

// Info logs at LevelInfo.
func Info(msg string, args ...any) {
	getLogger().Info(msg, args...)
}

// InfoContext logs at LevelInfo with the given context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	getLogger().InfoContext(ctx, msg, args...)
}

// Warn logs at LevelWarn.
func Warn(msg string, args ...any) {
	getLogger().Warn(msg, args...)
}

// WarnContext logs at LevelWarn with the given context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	getLogger().WarnContext(ctx, msg, args...)
}

// Error logs at LevelError.
func Error(msg string, args ...any) {
	getLogger().Error(msg, args...)
}

// ErrorContext logs at LevelError with the given context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	getLogger().ErrorContext(ctx, msg, args...)
}

// ErrAttr 错误类型Attr
func ErrAttr(err error) slog.Attr {
	return slog.String("err", err.Error())
}

// ReplaceSourceAttr source 格式化为 dir/file:line 格式
func ReplaceSourceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key != slog.SourceKey {
		return a
	}

	source, ok := a.Value.Any().(*slog.Source)
	if !ok {
		return a
	}

	dir, file := filepath.Split(source.File)
	source.File = filepath.Join(filepath.Base(dir), file)
	return a
}
