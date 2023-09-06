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
	"cmdDaemon/internal/tool"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"

	"go.uber.org/zap"
)

type IRegiser interface {
	Register() error
	Deregister() error
}

// consul represents a consul agent instance
// Consul implements IRegiser
type Consul struct {
	URL *url.URL // http://localhost:8500

	DC          string // datacenter, default: dc1
	Node        *Node
	ServiceList []*Service

	dcmds []*daemon.DaemonCmd // must runned dcmds

	logger *zap.SugaredLogger
}

// providers
func NewConsul(Consuladdr, dc string, node *Node, dcmds []*daemon.DaemonCmd, logger *zap.SugaredLogger) (*Consul, error) {
	serviceList := make([]*Service, 0, len(dcmds))
	url, err := url.Parse("http://" + Consuladdr)
	if err != nil {
		return nil, err
	}

	for _, dcmd := range dcmds {
		if dcmd.Status != 1 {
			continue
		}
		pidaddr, err := tool.PidAddr()
		if err != nil {
			return nil, err
		}

		port := parsePort(pidaddr[strconv.Itoa(dcmd.Cmd.Process.Pid)])
		svc, err := NewService(node.Name, dcmd.Cmd.Args[0], node.AdmIp, port)
		if err != nil {
			return nil, err
		}
		serviceList = append(serviceList, svc)
	}

	return &Consul{
		URL:         url,
		DC:          "dc1",
		Node:        node,
		ServiceList: serviceList,
		dcmds:       dcmds,
		logger:      logger,
	}, nil
}

func parsePort(addr string) string {
	return addr[strings.LastIndex(addr, ":")+1:]
}

// Only print consul config
func (c *Consul) PrintConf() {
	TmplStr := `
	{{$_len := len .}}
	{
		services: [
			{{range $index, $svc := .}}
			{
				"name": "{{$svc.Name}}",
				"port": {{$svc.Port}},
				"address": "{{$svc.Address}}"
			}{{if lt $index (sub $_len 1)}},{{end}}
			{{- end}}
		]
	}
	`

	services := c.ServiceList
	sub := func(a, b int) int {
		return a - b
	}

	tmpl, err := template.New("consul").Funcs(template.FuncMap{"sub": sub}).Parse(TmplStr)
	if err != nil {
		panic(err)
	}

	err = tmpl.Execute(os.Stdout, services)
	if err != nil {
		panic(err)
	}
}
