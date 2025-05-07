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
	"os"
	"os/exec"
	"testing"
	"text/template"
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
	node, _ := NewNode("localhost")
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

func TestConsul_PrintConf(t *testing.T) {
	services := []*Service{
		{Name: "prometheus", Port: "9090", IP: "localhost"},
		{Name: "grafana", Port: "3000", IP: "localhost"},
		{Name: "alertmanager", Port: "9093", IP: "localhost"},
		{Name: "node-exporter", Port: "9100", IP: "localhost"},
		{Name: "cadvisor", Port: "8080", IP: "localhost"},
		{Name: "consul", Port: "8500", IP: "localhost"},
	}
	err := PrintConf(services)
	if err != nil {
		t.Error(err)
	}
}

func PrintConf(svcs []*Service) error {
	TmplStr := `{{$_len := len .}}
{
	"services": [
		{{range $index, $svc := .}}
		{
			"name": "{{$svc.Name}}",
			"port": {{$svc.Port}},
			"address": "{{$svc.IP}}"
		}{{if lt $index (sub $_len 1)}},{{end}}
		{{- end}}
	]
}`

	out := os.Stdout
	sub := func(a, b int) int {
		return a - b
	}
	tmpl, err := template.New("consul").Funcs(template.FuncMap{"sub": sub}).Parse(TmplStr)
	if err != nil {
		return err
	}
	err = tmpl.Execute(out, svcs)
	if err != nil {
		return err
	}
	return nil
}
