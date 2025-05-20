package main

import (
	"context"

	"github.com/sq325/cmdDaemon/daemon"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func createLogger(level zapcore.Level) *zap.SugaredLogger {
	sugaredLogger := NewLogger(level)
	return sugaredLogger
}

func createDaemon(ctx context.Context, dcmds []*daemon.DaemonCmd, logger *zap.SugaredLogger) *daemon.Daemon {
	daemonDaemon := daemon.NewDaemon(ctx, dcmds, logger)
	return daemonDaemon
}
