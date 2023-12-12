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

// Log emits a log record with the current time and the given level and message.
// The Record's Attrs consist of the Logger's attributes followed by
// the Attrs specified by args.
//
// The attribute arguments are processed as follows:
//   - If an argument is an Attr, it is used as is.
//   - If an argument is a string and this is not the last argument,
//     the following argument is treated as the value and the two are combined
//     into an Attr.
//   - Otherwise, the argument is treated as a value with key "!BADKEY".
func Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	getLogger().Log(ctx, level, msg, args...)
}

// LogAttrs is a more efficient version of [Logger.Log] that accepts only Attrs.
func LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	getLogger().LogAttrs(ctx, level, msg, attrs...)
}

// With returns a Logger that includes the given attributes
// in each output operation. Arguments are converted to
// attributes as if by [Logger.Log].
func With(args ...any) *slog.Logger {
	return getLogger().With(args...)
}
