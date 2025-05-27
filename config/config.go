package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sq325/cmdDaemon/internal/tool"
	"gopkg.in/yaml.v2"
)

const (
	AnnotationsNameKey        = "name"        // 服务名称
	AnnotationsIPKey          = "ip"          // 管理IP
	AnnotationsPortKey        = "port"        // 端口号
	AnnotationsMetricsPathKey = "metricsPath" // metrics路径
	AnnotationsHostnameKey    = "hostname"    // 主机名
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
      name: "prometheus" # 默认basename cmd.Args[0]
      port: "9091" # 需要人工填写
      hostname: "proxy-a" # 默认os.Hostname()
      ip: "12.12.12.12" # 默认/etc/hosts中根据hostname查找
      metricsPath: "/metrics" # 需填写，如果为""，表示该cmd不提供metrics`
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
	conf.Accept(withIP)
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
	for i := range c.Cmds { // Iterate by index to modify the slice elements directly
		if c.Cmds[i].Annotations == nil {
			c.Cmds[i].Annotations = make(map[string]string, 10)
		}
		if c.Cmds[i].Annotations[AnnotationsHostnameKey] == "" {
			c.Cmds[i].Annotations[AnnotationsHostnameKey] = hostname
		}
	}
}

func withIP(c *Conf) {
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

	for i := range c.Cmds { // Iterate by index to modify the slice elements directly
		if c.Cmds[i].Annotations == nil {
			c.Cmds[i].Annotations = make(map[string]string, 10)
		}
		if c.Cmds[i].Annotations[AnnotationsIPKey] == "" {
			c.Cmds[i].Annotations[AnnotationsIPKey] = admIP
		}
	}
}

func withName(c *Conf) {
	if c == nil {
		return
	}

	for i := range c.Cmds { // Iterate by index to modify the slice elements directly
		if c.Cmds[i].Annotations == nil {
			c.Cmds[i].Annotations = make(map[string]string, 10)
		}
		if c.Cmds[i].Annotations[AnnotationsNameKey] == "" {
			// No need to check for nil again, already done above
			c.Cmds[i].Annotations[AnnotationsNameKey] = filepath.Base(c.Cmds[i].Cmd)
		}
	}
}
