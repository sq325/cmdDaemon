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
	"os"
	"text/template"

	"go.uber.org/zap"
)

// consul represents a consul agent instance
type Consul struct {
	DC          string // datacenter, default: dc1
	Node        *Node
	ServiceList []*Service

	dcmds []*daemon.DaemonCmd

	logger *zap.SugaredLogger
}

func NewConsul(dc string, dcmds []*daemon.DaemonCmd, logger *zap.SugaredLogger) (*Consul, error) {
	node, err := NewNode()
	if err != nil {
		return nil, err
	}
	return &Consul{
		DC:     "dc1",
		Node:   node,
		dcmds:  dcmds,
		logger: logger,
	}, nil
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

// consul service
type Service struct {
	Node    *Node
	Name    string
	Port    string
	Address string // ip
}

func NewService(node *Node, name, port, addr string) *Service {
	return &Service{
		Node:    node,
		Name:    name,
		Port:    port,
		Address: addr,
	}
}

/*
	{
	  "services": [
	    {
	      "name": "prometheus",
	      "port": 9090,
	      "address": "",
	      "checks": [
	        {
	          "name": "prometheusHealthy",
	          "http": "http://localhost:9090/-/healthy",
	          "interval": "30s"
	        }
	      ]
	    }
	  ]
	}
*/

// func consulConf() {
// 	services := []*Service{
// 		{Name: "prometheus", Port: "9090", Address: "localhost"},
// 		{Name: "grafana", Port: "3000", Address: "localhost"},
// 		{Name: "alertmanager", Port: "9093", Address: "localhost"},
// 		{Name: "node-exporter", Port: "9100", Address: "localhost"},
// 		{Name: "cadvisor", Port: "8080", Address: "localhost"},
// 		{Name: "consul", Port: "8500", Address: "localhost"},
// 	}

// 	sub := func(a, b int) int {
// 		return a - b
// 	}

// 	tmpl, err := template.New("consul").Funcs(template.FuncMap{"sub": sub}).Parse(TmplStr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	err = tmpl.Execute(os.Stdout, services)
// 	if err != nil {
// 		panic(err)
// 	}

// }
