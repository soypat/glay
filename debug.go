package glay

import (
	"context"
	"log/slog"
)

const (
	_traceLevel = slog.LevelDebug - 2
)

type logger struct {
	l *slog.Logger
}

var ctx = context.Background()

func (log logger) logattrs(lvl slog.Level, msg string, attrs ...slog.Attr) {
	if log.l != nil {
		log.l.LogAttrs(ctx, lvl, msg, attrs...)
	}
}

func (log logger) trace(msg string, attrs ...slog.Attr) { log.logattrs(_traceLevel, msg, attrs...) }
func (log logger) debug(msg string, attrs ...slog.Attr) { log.logattrs(slog.LevelDebug, msg, attrs...) }
func (log logger) info(msg string, attrs ...slog.Attr)  { log.logattrs(slog.LevelInfo, msg, attrs...) }
func (log logger) warn(msg string, attrs ...slog.Attr)  { log.logattrs(slog.LevelWarn, msg, attrs...) }
func (log logger) logerr(msg string, attrs ...slog.Attr) {
	log.logattrs(slog.LevelError, msg, attrs...)
}

func (log logger) SetLogger(logger *slog.Logger) {
	log.l = logger
}
