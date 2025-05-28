package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func newDcmds() []*DaemonCmd {
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
				AnnotationsNameKey:        "echo-service",
				AnnotationsIPKey:          "1.1.1.1",
				AnnotationsPortKey:        "1234",
				AnnotationsMetricsPathKey: "/metrics",
				AnnotationsHostnameKey:    "echo-host",
			},
		},
		{
			name:    "command with flags",
			cmdArgs: []string{"ls", "-l", "-a", "/tmp"},
			annotations: map[string]string{
				AnnotationsNameKey:        "ls-service",
				AnnotationsIPKey:          "2.2.2.2",
				AnnotationsPortKey:        "2345",
				AnnotationsMetricsPathKey: "/metrics",
				AnnotationsHostnameKey:    "ls-host",
			},
		},
		{
			name:    "command with no args",
			cmdArgs: []string{"pwd"},
			annotations: map[string]string{
				AnnotationsNameKey: "pwd-service",
				AnnotationsIPKey:   "3.3.3.3",
				AnnotationsPortKey: "3456",
			},
		},
		{
			name:    "command with args in specific order",
			cmdArgs: []string{"mycmd", "--opt1", "val1", "--opt2", "val2"},
			annotations: map[string]string{
				AnnotationsNameKey:        "mycmd-service",
				AnnotationsPortKey:        "4444",
				AnnotationsMetricsPathKey: "/api/v1/metrics",
			},
		},
	}

	dcmds := make([]*DaemonCmd, 0, len(tests))
	for _, tt := range tests {
		cmd := exec.Command(tt.cmdArgs[0], tt.cmdArgs[1:]...)
		dcmd := NewDaemonCmd(context.TODO(), cmd, tt.annotations)
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
							AnnotationsNameKey: "test-service",
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
							AnnotationsNameKey:        "test-service",
							AnnotationsIPKey:          "192.168.1.100",
							AnnotationsPortKey:        "8080",
							AnnotationsMetricsPathKey: "/metrics",
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
							AnnotationsNameKey:        "test-service",
							AnnotationsIPKey:          "192.168.1.100",
							AnnotationsPortKey:        "8080",
							AnnotationsMetricsPathKey: "/metrics",
							AnnotationsHostnameKey:    "proxy-a",
						},
						Status: Running,
					},
				},
			},
			expected: HttpSDResponse{
				{
					Targets: []string{"192.168.1.100:8080"},
					Labels: map[string]string{
						"name":                 "test-service",
						"hostAdmIp":            "192.168.1.100",
						"metricsPath":          "/metrics",
						AnnotationsHostnameKey: "proxy-a",
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
							AnnotationsNameKey:        "service-1",
							AnnotationsIPKey:          "192.168.1.100",
							AnnotationsPortKey:        "8080",
							AnnotationsMetricsPathKey: "/metrics",
							AnnotationsHostnameKey:    "proxy-a",
						},
						Status: Running,
					},
					{
						Annotations: map[string]string{
							AnnotationsNameKey:        "service-2",
							AnnotationsIPKey:          "192.168.1.101",
							AnnotationsPortKey:        "9090",
							AnnotationsMetricsPathKey: "/metrics",
							AnnotationsHostnameKey:    "proxy-b",
						},
						Status: Running,
					},
				},
			},
			expected: HttpSDResponse{
				{
					Targets: []string{"192.168.1.100:8080"},
					Labels: map[string]string{
						"name":                 "service-1",
						"hostAdmIp":            "192.168.1.100",
						"metricsPath":          "/metrics",
						AnnotationsHostnameKey: "proxy-a",
					},
				},
				{
					Targets: []string{"192.168.1.101:9090"},
					Labels: map[string]string{
						"name":                 "service-2",
						"hostAdmIp":            "192.168.1.101",
						"metricsPath":          "/metrics",
						AnnotationsHostnameKey: "proxy-b",
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
