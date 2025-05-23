package main

import (
	"context"
	"log/slog"

	"github.com/sq325/cmdDaemon/daemon"
)

func createDaemon(ctx context.Context, dcmds []*daemon.DaemonCmd, logger *slog.Logger) *daemon.Daemon {
	daemonDaemon := daemon.NewDaemon(ctx, dcmds, logger)
	return daemonDaemon
}
