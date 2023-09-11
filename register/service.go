package register

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type Service struct {
	NodeName string
	Name     string // 服务名称
	Port     any    // 服务的tcp port
	IP       string // 本机ip
}

func newService(nodeName, name, ip string, port any) (*Service, error) {
	var err error
	switch p := port.(type) {
	case int:
		if p < 0 || p > 65535 {
			err = errors.New("port must between 0 and 65535")
		}
	case string:
		if p == "" {
			err = errors.New("port must not be empty")
		}
		port, _ = strconv.Atoi(p) // transfer to int
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

func (svc *Service) ReqBody() ([]byte, error) {
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
	return bys, nil
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

