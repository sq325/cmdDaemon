package main

import (
	"cmdDaemon/config"
	"cmdDaemon/daemon"
	"cmdDaemon/register"
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

func createConsul(Consuladdr string, daemon2 *daemon.Daemon, intfList []string, logger *zap.SugaredLogger) (*register.Consul, error) {
	node, err := register.NewNode(intfList)
	if err != nil {
		return nil, err
	}
	v, err := register.NewServiceList(node, daemon2)
	if err != nil {
		return nil, err
	}
	consul, err := register.NewConsul(Consuladdr, node, daemon2, v, logger)
	if err != nil {
		return nil, err
	}
	return consul, nil
}
