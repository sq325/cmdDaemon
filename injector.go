package main

import (
	"cmdDaemon/config"
	"cmdDaemon/daemon"
	"context"
	"os/exec"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func createLogger(level zapcore.Level) *zap.SugaredLogger {
	sugaredLogger := NewLogger(level)
	return sugaredLogger
}

func createCmds(conf2 *config.Conf) []*exec.Cmd {
	v := config.GenerateCmds(conf2)
	return v
}

func createDaemon(ctx context.Context, cmds []*exec.Cmd, logger *zap.SugaredLogger) *daemon.Daemon {
	daemonDaemon := daemon.NewDaemon(ctx, cmds, logger)
	return daemonDaemon
}
