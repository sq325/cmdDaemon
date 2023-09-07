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
	"errors"
	"fmt"
	"net/http"
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
	URL    *url.URL // http://localhost:8500
	client *http.Client

	DC          string // datacenter, default: dc1
	Node        *Node
	ServiceList []*Service

	Daemon *daemon.Daemon

	logger *zap.SugaredLogger
}

// providers
func NewConsul(Consuladdr string, node *Node, daemon *daemon.Daemon, svcList []*Service, logger *zap.SugaredLogger) (*Consul, error) {
	url, err := url.Parse("http://" + Consuladdr)
	if err != nil {
		return nil, fmt.Errorf("url.Parse err: %w", err)
	}

	return &Consul{
		URL:         url,
		client:      &http.Client{},
		DC:          "dc1",
		Node:        node,
		ServiceList: svcList,
		Daemon:      daemon,
		logger:      logger,
	}, nil
}

func NewServiceList(node *Node, daemon *daemon.Daemon) ([]*Service, error) {
	dcmds := daemon.DCmds
	serviceList := make([]*Service, 0, len(dcmds))
	for _, dcmd := range dcmds {
		// 忽略已经退出的cmd
		if dcmd.Status != 1 {
			continue
		}
		pidaddr, err := tool.PidAddr()
		if err != nil {
			return nil, fmt.Errorf("PidAddr err: %w", err)
		}

		port := parsePort(pidaddr[strconv.Itoa(dcmd.Cmd.Process.Pid)])
		svc, err := newService(node.Name, dcmd.Cmd.Args[0], node.AdmIp, port)
		if err != nil {
			return nil, fmt.Errorf("NewService err: %w", err)
		}
		serviceList = append(serviceList, svc)
	}
	return serviceList, nil
}

func (c *Consul) Register() error {
	registerPath := "/v1/catalog/register"
	var errs error

	// 注册所有services
	for _, svc := range c.ServiceList {
		reader, err := svc.ReqBody()
		if err != nil {
			errs = errors.Join(errs, err)
		}
		req, err := http.NewRequest("PUT", c.URL.JoinPath(registerPath).String(), reader)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		req.Header.Set("Content-Type", "application/json")
		defer req.Body.Close()
		resp, err := c.client.Do(req)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		if resp.StatusCode != http.StatusOK {
			errs = errors.Join(errs, errors.New("register failed with status code: "+strconv.Itoa(resp.StatusCode)))
		}
	}
	return errs
}

func (c *Consul) Deregister() error {
	deregisterPath := "/v1/catalog/deregister"
	var errs error

	// 注销所有services
	for _, svc := range c.ServiceList {
		reader, err := svc.ReqBody()
		if err != nil {
			errs = errors.Join(errs, err)
		}
		req, err := http.NewRequest("PUT", c.URL.JoinPath(deregisterPath).String(), reader)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		req.Header.Set("Content-Type", "application/json")
		defer req.Body.Close()
		resp, err := c.client.Do(req)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		if resp.StatusCode != http.StatusOK {
			errs = errors.Join(errs, errors.New("deregister failed with status code: "+strconv.Itoa(resp.StatusCode)))
			c.logger.Errorln(resp.Body)
		}
	}
	return errs
}

// 更新serviceList
func (c *Consul) updateSvcList() {
	svcList, err := NewServiceList(c.Node, c.Daemon)
	if err != nil {
		c.logger.Errorln("NewServiceList err: ", err)
		return
	}
	c.ServiceList = svcList
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
