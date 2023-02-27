package config

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

var (
	configFile = "../Daemon.yml"
)

func setup() []byte {
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Println("Read config failed.")
	}
	return configBytes
}
func TestUnmarshal(t *testing.T) {
	configBytes := setup()
	conf, err := Unmarshal(configBytes)
	if err != nil {
		fmt.Println("Unmarshal config failed.")
	}
	cmds := GenerateCmds(conf)

	cc := exec.Command("./subapp", "-n", "3", "-i", "3s")
	t.Log(cc.Path, cc.Args)
	for _, c := range cmds {
		fmt.Println(c.Path, c.Args)
		c.Run()
	}
}
