package config

import (
	"testing"
)

func TestUnmarshalDefaultConfig(t *testing.T) {
	// Test with the DefaultConfig
	conf, err := Unmarshal([]byte(DefaultConfig))
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	if conf == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if conf.Cmds[0].Annotations["name"] != "prometheus" {
		t.Errorf("Expected name to be 'prometheus', but got: %s", conf.Cmds[0].Annotations["name"])
	}
}
func TestGenerateCmds(t *testing.T) {
	conf, err := Unmarshal([]byte(DefaultConfig))
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	cmds, _ := GenerateCmds(conf)
	if len(cmds) != 1 {
		t.Fatalf("Expected 1 command, but got: %d", len(cmds))
	}
	if cmds[0].Path != "./cmd/prometheusLinux/prometheus" {
		t.Errorf("Expected command path to be './cmd/prometheusLinux/prometheus', but got: %s", cmds[0].Path)
	}
	if len(cmds[0].Args[1:]) != 9 {
		t.Fatalf("Expected 9 arguments, but got: %d", len(cmds[0].Args))
	}
}
