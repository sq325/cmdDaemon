// Copyright 2023 Sun Quan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build wireinject
// +build wireinject

package main

import (
	"cmdDaemon/config"
	"cmdDaemon/daemon"
	"cmdDaemon/register"
	"context"
	"os/exec"

	"github.com/google/wire"
	"go.uber.org/zap"
)

// provider
var (
	DaemonSet = wire.NewSet(
		daemon.NewDaemon,
		daemon.NewLimiter,
		daemon.NewDaemonCmd,
	)
	ConsulSet = wire.NewSet(
		register.NewConsul,
		register.NewServiceList,
		register.NewNode,
	)
)

// injector
func createLogger() *zap.SugaredLogger {
	panic(wire.Build(NewLogger))
}

func createCmds(conf *config.Conf) []*exec.Cmd {
	panic(wire.Build(config.GenerateCmds))
}

func createDaemon(ctx context.Context, cmds []*exec.Cmd, logger *zap.SugaredLogger) *daemon.Daemon {
	panic(wire.Build(DaemonSet))
}

func createConsul(Consuladdr string, daemon *daemon.Daemon, logger *zap.SugaredLogger) (*register.Consul, error) {
	wire.Build(ConsulSet)
	return nil, nil
}
