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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"golang.org/x/exp/slog"
)

var defaultLogger atomic.Value

const (
	bannerLevel = slog.Level(1)
)

func init() {
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				source, ok := a.Value.Any().(*slog.Source)
				if !ok {
					return a
				}
				dir, file := filepath.Split(source.File)
				source.File = filepath.Join(filepath.Base(dir), file)
				return a
			}

			return a
		},
	})
	logger := slog.New(&handler{
		TextHandler: textHandler,
	})
	defaultLogger.Store(logger)
	logger.Log(context.Background(), bannerLevel, banner)
}

const (
	// LOGO is bk bscp inner banner.
	banner = `
===================================================================================
oooooooooo   oooo    oooo         oooooooooo     oooooooo     oooooo    oooooooooo
 888     Y8b  888   8P             888     Y8b d8P      Y8  d8P    Y8b   888    Y88
 888     888  888  d8              888     888 Y88bo       888           888    d88
 888oooo888   88888[      8888888  888oooo888     Y8888o   888           888ooo88P
 888     88b  888 88b              888     88b        Y88b 888           888
 888     88P  888   88b            888     88P oo      d8P  88b    ooo   888
o888bood8P   o888o  o888o         o888bood8P   88888888P     Y8bood8P   o888o
===================================================================================`
)

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

	if rr.Level == bannerLevel {
		fmt.Println(strings.TrimSpace(rr.Message))
		return nil
	}

	return h.TextHandler.Handle(ctx, *rr)
}

func getLogger() *slog.Logger {
	return defaultLogger.Load().(*slog.Logger)
}

// SetHandler set logger
func SetHandler(handler slog.Handler) {
	defaultLogger.Store(slog.New(handler))
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

// With returns a Logger that includes the given attributes
// in each output operation. Arguments are converted to
// attributes as if by [Logger.Log].
func With(args ...any) *slog.Logger {
	return getLogger().With(args...)
}

// ErrAttr ..
func ErrAttr(err error) slog.Attr {
	return slog.String("err", err.Error())
}
