package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
func TestDaemonCmd_startAndWait(t *testing.T) {
	t.Run("successful command execution", func(t *testing.T) {
		ctx := context.Background()
		cmd := exec.Command("echo", "hello")
		annotations := map[string]string{"name": "test", "port": "8080"}
		dcmd := NewDaemonCmd(ctx, cmd, annotations)

		ch := make(chan *DaemonCmd, 1)

		go dcmd.startAndWait(ch)

		select {
		case result := <-ch:
			if result.Status != Exited {
				t.Errorf("Expected status %d, got %d", Exited, result.Status)
			}
			if result.Err != nil {
				t.Errorf("Expected no error, got %v", result.Err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out")
		}
	})

	t.Run("command start failure", func(t *testing.T) {
		ctx := context.Background()
		cmd := exec.Command("nonexistent-command")
		dcmd := NewDaemonCmd(ctx, cmd, nil)

		ch := make(chan *DaemonCmd, 1)

		go dcmd.startAndWait(ch)

		select {
		case result := <-ch:
			if result.Err == nil {
				t.Error("Expected error for nonexistent command")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.Command("sleep", "10")
		dcmd := NewDaemonCmd(ctx, cmd, nil)

		ch := make(chan *DaemonCmd, 1)

		go dcmd.startAndWait(ch)
		cancel()

		select {
		case <-ch:
			t.Error("Should not receive on channel when context is cancelled")
		case <-time.After(100 * time.Millisecond):
			// Expected behavior
		}
	})

	t.Run("with log directory", func(t *testing.T) {
		ctx := context.Background()
		cmd := exec.Command("echo", "test log")
		annotations := map[string]string{"name": "logtest", "port": "9090"}
		dcmd := NewDaemonCmd(ctx, cmd, annotations)

		tmpDir := t.TempDir()
		dcmd.logDir = tmpDir

		ch := make(chan *DaemonCmd, 1)

		go dcmd.startAndWait(ch)

		select {
		case result := <-ch:
			if result.Status != Exited {
				t.Errorf("Expected status %d, got %d", Exited, result.Status)
			}
			if result.Err != nil {
				t.Errorf("Expected no error, got %v", result.Err)
			}

			// Check if log file was created
			expectedLogFile := filepath.Join(tmpDir, fmt.Sprintf("logtest_9090_%s.log", dcmd.CmdHash()))
			if _, err := os.Stat(expectedLogFile); os.IsNotExist(err) {
				t.Errorf("Log file %s was not created", expectedLogFile)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out")
		}
	})

	t.Run("log directory creation failure", func(t *testing.T) {
		ctx := context.Background()
		cmd := exec.Command("echo", "test")
		annotations := map[string]string{"name": "test", "port": "8080"}
		dcmd := NewDaemonCmd(ctx, cmd, annotations)

		// Use an invalid path that cannot be created
		dcmd.logDir = "/invalid/path/that/cannot/be/created"

		ch := make(chan *DaemonCmd, 1)

		dcmd.startAndWait(ch)

		if dcmd.Err == nil {
			t.Error("Expected error when creating invalid log directory")
		}
	})
}
