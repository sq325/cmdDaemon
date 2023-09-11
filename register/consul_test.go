// Copyright 2023 Sun Quan
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package register

import (
	"cmdDaemon/daemon"
	"context"
	"net"
	"net/url"
	"os/exec"
	"testing"
)

func TestSplitHostPort(t *testing.T) {
	host, port, _ := net.SplitHostPort("*:8080")
	t.Log(port)
	t.Log(host)
}

func TestUrlJoin(t *testing.T) {
	deregisterPath := "/v1/catalog/deregister/"
	url, err := url.Parse("http://" + "localhost:8500")
	if err != nil {
		t.Error(err)
	}
	t.Log(url.String())
	t.Log(url.JoinPath(deregisterPath).String())

}

func TestNewServiceList(t *testing.T) {
	node, _ := NewNode([]string{"en0"})
	cmd1 := exec.Command("./prometheus", "--config.file=prometheus.yml", "--web.listen-address=:8080")
	cmd1.Process.Pid = 79704
	cmd2 := exec.Command("./prometheus", "--config.file=prometheus.yml", "--web.listen-address=:8081")
	cmd1.Process.Pid = 79705
	cmds := make([]*exec.Cmd, 0, 2)
	cmds = append(cmds, cmd1)
	cmds = append(cmds, cmd2)
	ctx := context.Background()
	_daemon := daemon.NewDaemon(ctx, cmds, nil)
	NewServiceList(node, _daemon)

}
