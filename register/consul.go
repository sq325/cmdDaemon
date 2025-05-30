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
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/sq325/cmdDaemon/daemon"
	"github.com/sq325/cmdDaemon/internal/tool"
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

	logger *slog.Logger // Changed from *zap.SugaredLogger
}

// providers
func NewConsul(Consuladdr string, node *Node, daemon *daemon.Daemon, svcList []*Service, logger *slog.Logger) (*Consul, error) { // Changed logger type
	if Consuladdr == "" {
		return nil, errors.New("consuladdr is empty")
	}
	url, err := url.Parse("http://" + Consuladdr)
	if err != nil {
		return nil, fmt.Errorf("url.Parse err: %w", err)
	}

	return &Consul{
		URL: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
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
	var errs error
	for _, dcmd := range dcmds {
		// 忽略已经退出的cmd
		if dcmd.Status != 1 {
			errors.Join(errs, errors.New("ignore exited cmd: "+dcmd.Cmd.String()))
			continue
		}
		pidaddr, err := tool.PidAddr()
		if err != nil {
			errors.Join(errs, fmt.Errorf("PidAddr err: %w", err))
			return nil, errs
		}

		if dcmd.Cmd.Process == nil {
			errs = errors.Join(errs, errors.New("cmd.Process is nil: "+dcmd.Cmd.String()))
			continue
		}
		pid := strconv.Itoa(dcmd.Cmd.Process.Pid)
		addr, ok := pidaddr[pid] // *:59869
		if !ok {
			errs = errors.Join(errs, errors.New("pidaddr not found: "+dcmd.Cmd.String()))
			continue
		}
		port := tool.Parseport(addr) // 59869

		svc, err := newService(node.Name, svcName(filepath.Base(dcmd.Cmd.Args[0]), port, pid), node.AdmIp, port) // 防止svc重名-> name:port or name@pid
		if err != nil {
			errors.Join(errs, fmt.Errorf("NewService err: %w", err))
			return nil, errs
		}
		serviceList = append(serviceList, svc)
	}
	return serviceList, errs
}

func (c *Consul) Register() error {
	registerPath := "/v1/catalog/register"
	var errs error

	// 注册所有services
	for _, svc := range c.ServiceList {
		bys, err := svc.RegReqBody()
		if err != nil {
			errs = errors.Join(errs, err)
		}
		c.logger.Debug(fmt.Sprintf("register req body: %s", string(bys))) // Changed from Debugln
		reader := bytes.NewReader(bys)
		req, err := http.NewRequest("PUT", c.URL.JoinPath(registerPath).String(), reader)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.logger.Debug(fmt.Sprintf("register req: %+v", req)) // Changed from Debugf
		defer req.Body.Close()
		resp, err := c.client.Do(req)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		// defer c.client.CloseIdleConnections()
		c.logger.Debug(fmt.Sprintf("register resp: %v", resp)) // Changed from Debugln
		if resp != nil && resp.StatusCode != http.StatusOK {
			errs = errors.Join(errs, errors.New("register failed with status code: "+strconv.Itoa(resp.StatusCode)))
			var body []byte
			resp.Body.Read(body) // Note: This Read might not fill body as expected. Consider io.ReadAll
			bodystr := string(body)
			c.logger.Debug(fmt.Sprintf("register failed, resp body: %s", bodystr)) // Changed from Debugln
		}
		c.logger.Info(fmt.Sprintf("Register service: %+v successfully", svc)) // Changed from Infof
	}
	return errs
}

func (c *Consul) Deregister() error {
	deregisterPath := "/v1/catalog/deregister"
	var errs error

	// 注销所有services
	for _, svc := range c.ServiceList {
		bys, err := svc.DeregReqBody()
		if err != nil {
			errs = errors.Join(errs, err)
		}
		c.logger.Debug(fmt.Sprintf("deregister req body: %s", string(bys))) // Changed from Debugln
		reader := bytes.NewReader(bys)
		req, err := http.NewRequest("PUT", c.URL.JoinPath(deregisterPath).String(), reader)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.logger.Debug(fmt.Sprintf("deregister req: %v", req)) // Changed from Debugln
		defer req.Body.Close()
		resp, err := c.client.Do(req)
		c.logger.Debug(fmt.Sprintf("deregister resp: %+v", resp)) // Changed from Debugf
		if err != nil {
			errs = errors.Join(errs, err)
		}
		// defer c.client.CloseIdleConnections()
		if resp != nil && resp.StatusCode != http.StatusOK {
			errs = errors.Join(errs, errors.New("deregister failed with status code: "+strconv.Itoa(resp.StatusCode)))
			var body []byte
			resp.Body.Read(body) // Note: This Read might not fill body as expected. Consider io.ReadAll
			bodystr := string(body)
			c.logger.Debug(fmt.Sprintf("deregister failed, resp body: %s", bodystr)) // Changed from Debugln
		}
		c.logger.Info(fmt.Sprintf("Deregister service: %+v successfully", svc)) // Changed from Infof
	}
	return errs
}

// Watch watch daemon.Dcmds status change and update serviceList
func (c *Consul) Watch() {
	var wg sync.WaitGroup

	// 判断runningCount和exitedCount的数量是否有变化
	ticker1m := time.NewTicker(1 * time.Minute)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runningCount := c.Daemon.GetRunningCmdLen()
		exitedCount := c.Daemon.GetExitedCmdLen()
		for range ticker1m.C {
			if runningCount != c.Daemon.GetRunningCmdLen() || exitedCount != c.Daemon.GetExitedCmdLen() {
				if err := c.RegisterAgain(); err != nil {
					c.logger.Error(fmt.Sprintf("RegisterAgain err: %v", err)) // Changed from Errorln
					continue
				}
				runningCount = c.Daemon.GetRunningCmdLen()
				exitedCount = c.Daemon.GetExitedCmdLen()
			}
		}
	}()

	// 15mins 重新register一次
	ticker15m := time.NewTicker(15 * time.Minute)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range ticker15m.C {
			if err := c.RegisterAgain(); err != nil {
				c.logger.Error(fmt.Sprintf("RegisterAgain err: %v", err)) // Changed from Errorln
				continue
			}
		}
	}()

	wg.Wait()
}

func (c *Consul) RegisterAgain() (errs error) {
	if err := c.Updatesvclist(); err != nil {
		return fmt.Errorf("Updatesvclist err: %w, don't RegisterAgain", err)
	}
	if err := c.Deregister(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.Register(); err != nil {
		errs = errors.Join(errs, err)
		return errs
	}
	return errs
}

// 更新serviceList
func (c *Consul) Updatesvclist() error {
	svcList, err := NewServiceList(c.Node, c.Daemon)
	if err != nil {
		return err
	}
	c.ServiceList = svcList
	return nil
}

// Only print consul config
func (c *Consul) PrintConf(out io.Writer) {
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

	services := c.ServiceList
	sub := func(a, b int) int {
		return a - b
	}
	tmpl, err := template.New("consul").Funcs(template.FuncMap{"sub": sub}).Parse(TmplStr)
	if err != nil {
		c.logger.Error(fmt.Sprintf("Parse consul config failed. %v", err)) // Changed from Errorln
	}
	err = tmpl.Execute(out, services)
	if err != nil {
		c.logger.Error(fmt.Sprintf("Execute consul config failed. %v", err)) // Changed from Errorln
	}
}

func svcName(name, port, pid string) string {
	if port != "" {
		return name + ":" + port
	}
	return name + "@" + pid
}
