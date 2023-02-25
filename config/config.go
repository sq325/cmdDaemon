package config

import (
	"fmt"
	"os/exec"

	"gopkg.in/yaml.v2"
)

var (
	DefaultConfig = `cmds:
	- cmd: ./prometheus
		args: 
		- --web.listen-address="0.0.0.0:9091"
		- --web.config.file="./prometheus.yml"
		- --web.enable-lifecycle
		- --storage.tsdb.path="./data1/"
		- --storage.tsdb.retention=7d`
)

type Conf struct {
	Cmds []struct {
		Cmd  string   `yaml:"cmd"`
		Args []string `yaml:"args"`
	} `yaml:"cmds"`
}

type CmdGenerator interface {
	Generate() []*exec.Cmd
}

func NewCmdGenerator() CmdGenerator {
	return &prometheusGenerate{}
}

type prometheusGenerate struct {
}

func (prom *prometheusGenerate) Generate() []*exec.Cmd {
	return nil
}

func Unmarshal(b []byte) (*Conf, error) {
	var conf Conf
	err := yaml.Unmarshal(b, &conf)
	if err != nil {
		fmt.Println("Unmarshal config failed")
		panic(err)
	}
	return &conf, err
}
