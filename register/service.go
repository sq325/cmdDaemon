package register

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

type Service struct {
	NodeName string
	Name     string // 服务名称
	Port     any    // 服务的tcp port
	IP       string // 本机ip
}

func NewService(nodeName, name, ip string, port any) (*Service, error) {
	var err error
	switch port.(type) {
	case int:
		if port.(int) < 0 || port.(int) > 65535 {
			err = errors.New("port must between 0 and 65535")
		}
	case string:
		if port.(string) == "" {
			err = errors.New("port must not be empty")
		}
		port, _ = strconv.Atoi(port.(string)) // transfer to int
	default:
		err = errors.New("port must be int or string")
	}

	return &Service{
		NodeName: nodeName,
		Name:     name,
		Port:     port,
		IP:       ip,
	}, err
}

func (svc *Service) ReqBody() (io.Reader, error) {
	requestJson := struct {
		NodeName       string `json:"Node"`
		SkipNodeUpdate bool   `json:"SkipNodeUpdate,omitempty"`
		Address        string `json:"Address,omitempty"`
		Service        struct {
			Service string `json:"Service"`
			Port    int    `json:"Port"`
			Address string `json:"Address,omitempty"`
		} `json:"Service"`
	}{
		NodeName: svc.NodeName,
		Address:  svc.IP,
		Service: struct {
			Service string `json:"Service"`
			Port    int    `json:"Port"`
			Address string `json:"Address,omitempty"`
		}{
			Service: svc.Name,
			Port:    svc.Port.(int),
			Address: svc.IP,
		},
	}

	bys, err := json.Marshal(requestJson)
	if err != nil {
		return nil, err
	}
	if len(bys) == 0 {
		return nil, fmt.Errorf("request body is empty, service: %v", svc)
	}
	reader := bytes.NewReader(bys)
	return reader, nil
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
