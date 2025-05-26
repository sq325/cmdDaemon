package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sq325/cmdDaemon/internal/tool"
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
      - --storage.tsdb.retention.time
      - 7d
    annotations:
      name: "prometheus"
      port: "9091"
      hostname: "proxy-a"
      admIP: "12.12.12.12"
      metricsPath: "/metrics"`
)

type Conf struct {
	Cmds []struct {
		Cmd  string   `yaml:"cmd"`
		Args []string `yaml:"args"`
		// Annotations for the command, such as name, port, hostname, admIP, etc.
		// port must be set if the command listens on a port
		// if no metrics, set metricsPath to ""
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"cmds"`
}

func (c *Conf) Accept(v confVisitor) {
	if v == nil {
		return
	}
	v(c)
}

func GenerateCmds(conf *Conf) ([]*exec.Cmd, []map[string]string) {
	if len(conf.Cmds) == 0 {
		return nil, nil
	}

	cmds := make([]*exec.Cmd, 0, len(conf.Cmds))
	annotationsList := make([]map[string]string, 0, len(conf.Cmds))

	conf.Accept(withHostName)
	conf.Accept(withAdmIp)
	conf.Accept(withName)

	for _, cmd := range conf.Cmds {
		cmds = append(cmds, exec.Command(cmd.Cmd, cmd.Args...))
		annotationsList = append(annotationsList, cmd.Annotations)
	}
	return cmds, annotationsList
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

type confVisitor func(conf *Conf)

func withHostName(c *Conf) {
	if c == nil {
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		fmt.Printf("Error getting hostname: %v\n", err)
		return
	}
	for _, cmd := range c.Cmds {
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string, 10)
		}
		if cmd.Annotations["hostname"] == "" {
			cmd.Annotations["hostname"] = hostname
		}
	}
}

func withAdmIp(c *Conf) {
	if c == nil {
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println("Error getting hostname:", err)
		return
	}
	admIP, err := tool.IpFromHostname(hostname)
	if err != nil {
		fmt.Printf("Error getting IP from hostname: %v\n", err)
		return
	}

	for _, cmd := range c.Cmds {
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string, 10)
		}
		if cmd.Annotations["admIP"] == "" {
			cmd.Annotations["admIP"] = admIP
		}
	}
}

func withName(c *Conf) {
	if c == nil {
		return
	}

	for _, cmd := range c.Cmds {
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string, 10)
		}
		if cmd.Annotations["name"] == "" {
			if cmd.Annotations == nil {
				cmd.Annotations = make(map[string]string, 10)
			}
			cmd.Annotations["name"] = filepath.Base(cmd.Cmd)
		}
	}
}
