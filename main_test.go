package main

import (
	"os/exec"
	"testing"
)

func Test_killcmd(t *testing.T) {
	cmd := exec.Command("./prometheus", "--web.listen-address", "0.0.0.0:9091")
	err := killcmd(cmd)
	t.Log(err)
}
