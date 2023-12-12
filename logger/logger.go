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
	"log/slog"
	"sync/atomic"
)

var defaultLogger atomic.Value

func init() {
	defaultLogger.Store(slog.Default())
}

func getLogger() *slog.Logger {
	return defaultLogger.Load().(*slog.Logger)
}

// SetLogger set logger
func SetLogger(logger *slog.Logger) {
	defaultLogger.Store(logger)
}

// Debug
func Debug(msg string, args ...any) {
	getLogger().Debug(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	getLogger().DebugContext(ctx, msg, args...)
}

func Info(msg string, args ...any) {
	getLogger().Info(msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	getLogger().InfoContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	getLogger().Warn(msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	getLogger().WarnContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	getLogger().Error(msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	getLogger().ErrorContext(ctx, msg, args...)
}

func Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	getLogger().Log(ctx, level, msg, args...)
}

func LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	getLogger().LogAttrs(ctx, level, msg, attrs...)
}

// With calls Logger.With on the default logger.
func With(args ...any) *slog.Logger {
	return getLogger().With(args...)
}
