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

package tool

import (
	"cmdDaemon/daemon"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// AddrCmdMap return a map of port and cmd
// addr is raw lsof ip:port column
func AddrCmdMap(dcmds []*daemon.DaemonCmd) (map[string]string, error) {
	if len(dcmds) == 0 {
		return nil, nil
	}
	var addrCmd = make(map[string]string, len(dcmds))
	pidAddr, err := PidAddr()
	if err != nil {
		return nil, fmt.Errorf("pidAddr err: %w", err)
	}

	for _, dcmd := range dcmds {
		if dcmd.Cmd == nil || dcmd.Cmd.Process == nil {
			continue
		}
		pid := strconv.Itoa(dcmd.Cmd.Process.Pid)

		if addr, ok := pidAddr[pid]; ok {
			addrCmd[addr] = dcmd.Cmd.String()
		}
	}

	return addrCmd, nil
}

// pidAddr return a map of pid: addr(host:port)
func PidAddr() (map[string]string, error) {
	var spacePattern = regexp.MustCompile(`\s+`)

	out, err := exec.Command("lsof", "-Pi", "TCP", "-s", "TCP:LISTEN").Output()
	if err != nil {
		return nil, fmt.Errorf("lsof err: %v", err)
	}
	outS := strings.Split(string(out), "\n")
	var pidAddr = make(map[string]string, len(outS))
	for _, line := range outS {
		lineSlice := spacePattern.Split(line, -1)
		if len(lineSlice) < 2 {
			continue
		}
		addr := lineSlice[len(lineSlice)-2]
		pid := lineSlice[1]
		pidAddr[pid] = addr
	}
	return pidAddr, nil
}
