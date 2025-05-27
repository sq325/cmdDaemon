package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/sq325/cmdDaemon/config"
)

func newDcmds(annotations map[string]string) []*DaemonCmd {
	type testCase struct {
		name        string
		cmdArgs     []string // First element is command, rest are args
		annotations map[string]string
	}

	tests := []testCase{
		{
			name:    "simple command",
			cmdArgs: []string{"echo", "hello", "world"},
			annotations: map[string]string{
				config.AnnotationsNameKey:        "echo-service",
				config.AnnotationsIPKey:          "1.1.1.1",
				config.AnnotationsPortKey:        "1234",
				config.AnnotationsMetricsPathKey: "/metrics",
				config.AnnotationsHostnameKey:    "echo-host",
			},
		},
		{
			name:    "command with flags",
			cmdArgs: []string{"ls", "-l", "-a", "/tmp"},
			annotations: map[string]string{
				config.AnnotationsNameKey:        "ls-service",
				config.AnnotationsIPKey:          "2.2.2.2",
				config.AnnotationsPortKey:        "2345",
				config.AnnotationsMetricsPathKey: "/metrics",
				config.AnnotationsHostnameKey:    "ls-host",
			},
		},
		{
			name:    "command with no args",
			cmdArgs: []string{"pwd"},
			annotations: map[string]string{
				config.AnnotationsNameKey: "pwd-service",
				config.AnnotationsIPKey:   "3.3.3.3",
				config.AnnotationsPortKey: "3456",
			},
		},
		{
			name:    "command with args in specific order",
			cmdArgs: []string{"mycmd", "--opt1", "val1", "--opt2", "val2"},
			annotations: map[string]string{
				config.AnnotationsNameKey:        "mycmd-service",
				config.AnnotationsPortKey:        "4444",
				config.AnnotationsMetricsPathKey: "/api/v1/metrics",
			},
		},
	}

	dcmds := make([]*DaemonCmd, 0, len(tests))
	for _, tt := range tests {
		cmd := exec.Command(tt.cmdArgs[0], tt.cmdArgs[1:]...)
		dcmd := NewDaemonCmd(nil, cmd, tt.annotations)
		dcmds = append(dcmds, dcmd)
	}
	return dcmds
}

func TestHttpSDHandler(t *testing.T) {
	tests := []struct {
		name     string
		daemon   *Daemon
		expected HttpSDResponse
	}{
		{
			name: "empty daemon",
			daemon: &Daemon{
				DCmds: []*DaemonCmd{},
			},
			expected: HttpSDResponse{},
		},
		{
			name: "daemon with nil annotations",
			daemon: &Daemon{
				DCmds: []*DaemonCmd{
					{
						Annotations: nil,
						Status:      Running,
					},
				},
			},
			expected: HttpSDResponse{},
		},
		{
			name: "daemon with missing required annotations",
			daemon: &Daemon{
				DCmds: []*DaemonCmd{
					{
						Annotations: map[string]string{
							config.AnnotationsNameKey: "test-service",
						},
						Status: Running,
					},
				},
			},
			expected: HttpSDResponse{},
		},
		{
			name: "daemon with non-running status",
			daemon: &Daemon{
				DCmds: []*DaemonCmd{
					{
						Annotations: map[string]string{
							config.AnnotationsNameKey:        "test-service",
							config.AnnotationsIPKey:          "192.168.1.100",
							config.AnnotationsPortKey:        "8080",
							config.AnnotationsMetricsPathKey: "/metrics",
						},
						Status: Exited,
					},
				},
			},
			expected: HttpSDResponse{},
		},
		{
			name: "daemon with valid running service",
			daemon: &Daemon{
				DCmds: []*DaemonCmd{
					{
						Annotations: map[string]string{
							config.AnnotationsNameKey:        "test-service",
							config.AnnotationsIPKey:          "192.168.1.100",
							config.AnnotationsPortKey:        "8080",
							config.AnnotationsMetricsPathKey: "/metrics",
						},
						Status: Running,
					},
				},
			},
			expected: HttpSDResponse{
				{
					Targets: []string{"192.168.1.100:8080"},
					Labels: map[string]string{
						"name":      "test-service",
						"hostAdmIp": "192.168.1.100",
					},
				},
			},
		},
		{
			name: "daemon with multiple services",
			daemon: &Daemon{
				DCmds: []*DaemonCmd{
					{
						Annotations: map[string]string{
							config.AnnotationsNameKey:        "service-1",
							config.AnnotationsIPKey:          "192.168.1.100",
							config.AnnotationsPortKey:        "8080",
							config.AnnotationsMetricsPathKey: "/metrics",
						},
						Status: Running,
					},
					{
						Annotations: map[string]string{
							config.AnnotationsNameKey:        "service-2",
							config.AnnotationsIPKey:          "192.168.1.101",
							config.AnnotationsPortKey:        "9090",
							config.AnnotationsMetricsPathKey: "/metrics",
						},
						Status: Running,
					},
				},
			},
			expected: HttpSDResponse{
				{
					Targets: []string{"192.168.1.100:8080"},
					Labels: map[string]string{
						"name":      "service-1",
						"hostAdmIp": "192.168.1.100",
					},
				},
				{
					Targets: []string{"192.168.1.101:9090"},
					Labels: map[string]string{
						"name":      "service-2",
						"hostAdmIp": "192.168.1.101",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := HttpSDHandler(tt.daemon)
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}

			var response HttpSDResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if len(response) != len(tt.expected) {
				t.Errorf("expected %d target groups, got %d. Got response: %+v", len(tt.expected), len(response), response)
				return
			}

			for i, expectedTg := range tt.expected {
				if i >= len(response) {
					t.Errorf("target group %d: expected but not found", i)
					continue
				}
				actualTg := response[i]

				if len(actualTg.Targets) != len(expectedTg.Targets) {
					t.Errorf("target group %d: expected %d targets, got %d. Expected: %v, Got: %v", i, len(expectedTg.Targets), len(actualTg.Targets), expectedTg.Targets, actualTg.Targets)
					continue
				}
				for j, target := range expectedTg.Targets {
					if actualTg.Targets[j] != target {
						t.Errorf("target group %d, target %d: expected %s, got %s", i, j, target, actualTg.Targets[j])
					}
				}

				if len(actualTg.Labels) != len(expectedTg.Labels) {
					t.Errorf("target group %d: expected %d labels, got %d. Expected: %v, Got: %v", i, len(expectedTg.Labels), len(actualTg.Labels), expectedTg.Labels, actualTg.Labels)
					continue
				}
				for key, value := range expectedTg.Labels {
					if actualTg.Labels[key] != value {
						t.Errorf("target group %d, label %s: expected %s, got %s", i, key, value, actualTg.Labels[key])
					}
				}
			}
		})
	}
}
