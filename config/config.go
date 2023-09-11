package config

import (
	"fmt"
	"os/exec"

	"gopkg.in/yaml.v2"
)

var (
	DefaultConfig = `cmds:
  - cmd: ./cmd/prometheusLinux/prometheus
    args: 
    - --web.listen-address
    - "0.0.0.0:9091"
    - --config.file
    - "./cmd/prometheusLinux/prometheus.yml"
    - --web.enable-lifecycle
    - --storage.tsdb.path
    - "./cmd/prometheusLinux/data1/"
    - --storage.tsdb.retention
    - 7d`
)

type Conf struct {
	Cmds []struct {
		Cmd  string   `yaml:"cmd"`
		Args []string `yaml:"args"`
	} `yaml:"cmds"`
}

func GenerateCmds(conf *Conf) []*exec.Cmd {
	if len(conf.Cmds) == 0 {
		return nil
	}

	cmds := make([]*exec.Cmd, 0, len(conf.Cmds))
	for _, cmd := range conf.Cmds {
		cmds = append(cmds, exec.Command(cmd.Cmd, cmd.Args...))
	}
	return cmds
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
