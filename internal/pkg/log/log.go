// Package log provides an abstraction over the standard libraries "log/slog" that is tied
// to a [context.Context].
package log

import (
	"context"
	"log/slog"
)

type loggerKeyType = struct{}

var loggerKey = loggerKeyType{}

func New(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l) //nolint:staticcheck // SA1029: false positive
}

func getLogger(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return l
}

func Warn(ctx context.Context, msg string, args ...any) {
	getLogger(ctx).WarnContext(ctx, msg, args...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	getLogger(ctx).DebugContext(ctx, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	getLogger(ctx).InfoContext(ctx, msg, args...)
}

func Attr[T interface{ string | int | []string }](key string, value T) slog.Attr {
	switch value := any(value).(type) {
	case string:
		return slog.String(key, value)
	case int:
		return slog.Int(key, value)
	case []string:
		return slog.Any(key, value)
	default:
		panic("impossible")
	}
}
