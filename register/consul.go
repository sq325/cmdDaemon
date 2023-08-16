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
  "os"
  "text/template"
)

type Consul struct {
  ServerAddr  string
  Node        *Node
  ServiceList []*Service
}

func NewConsul(addr string, node *Node, services []*Service) *Consul {
  return &Consul{
    ServerAddr:  addr,
    Node:        node,
    ServiceList: services,
  }
}

type Node struct {
  Name    string
  Address string
}

func NewNode(name, add string) *Node {
  return &Node{
    Name:    name,
    Address: add,
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

func (s *Service) Register() {

}

func (s *Service) Deregister() {

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
var TmplStr = `
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

func consulConf() {
  services := []*Service{
    {Name: "prometheus", Port: "9090", Address: "localhost"},
    {Name: "grafana", Port: "3000", Address: "localhost"},
    {Name: "alertmanager", Port: "9093", Address: "localhost"},
    {Name: "node-exporter", Port: "9100", Address: "localhost"},
    {Name: "cadvisor", Port: "8080", Address: "localhost"},
    {Name: "consul", Port: "8500", Address: "localhost"},
  }

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