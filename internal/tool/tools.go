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
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

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

func Parseport(addr string) string {
	return addr[strings.LastIndex(addr, ":")+1:]
}

func Hostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, err
}

func IpFromHostname(hostname string) (string, error) {
	file, err := os.Open("/etc/hosts")
	if err != nil {
		return "", fmt.Errorf("无法打开 /etc/hosts 文件: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// 忽略注释和空行
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := fields[0]
		for i := 1; i < len(fields); i++ {
			if fields[i] == hostname {
				return ip, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取 /etc/hosts 文件时出错: %v", err)
	}

	return "", fmt.Errorf("在 /etc/hosts 文件中未找到主机名: %s", hostname)
}
