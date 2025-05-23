package daemon

import (
	"context"
	"os/exec"
	"testing"

	"github.com/sq325/cmdDaemon/internal/tool"
)

func TestDaemonCmd_CmdHash(t *testing.T) {
	type testCase struct {
		name    string
		cmdArgs []string // First element is command, rest are args
	}

	tests := []testCase{
		{
			name:    "simple command",
			cmdArgs: []string{"echo", "hello", "world"},
		},
		{
			name:    "command with flags",
			cmdArgs: []string{"ls", "-l", "-a", "/tmp"},
		},
		{
			name:    "command with no args",
			cmdArgs: []string{"pwd"},
		},
		{
			name:    "command with args in specific order",
			cmdArgs: []string{"mycmd", "--opt1", "val1", "--opt2", "val2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(tt.cmdArgs[0], tt.cmdArgs[1:]...)
			dcmd := NewDaemonCmd(context.Background(), cmd, nil)

			got := dcmd.CmdHash()
			want := tool.HashCmd(cmd) // Expected value is what tool.HashCmd produces
			t.Logf("CmdHash() for cmd '%s' \nhash= %v", cmd.String(), got)
			if got != want {
				t.Errorf("DaemonCmd.CmdHash() for cmd '%s' = %v, want %v", cmd.String(), got, want)
			}
		})
	}
}
