package daemon

import (
	"context"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestDaemon_RegisterMetrics(t *testing.T) {
	tests := []struct {
		name        string
		registerer  prometheus.Registerer
		wantErr     bool
		expectedErr string
	}{
		{
			name:        "nil registerer",
			registerer:  nil,
			wantErr:     true,
			expectedErr: "prometheus registerer is nil",
		},
		{
			name:       "valid registerer",
			registerer: prometheus.NewRegistry(),
			wantErr:    false,
		},
		{
			name: "registerer with existing collector",
			registerer: func() prometheus.Registerer {
				r := prometheus.NewRegistry()
				d := &Daemon{
					ctx:    context.Background(),
					DCmds:  []*DaemonCmd{},
					Logger: slog.Default(),
				}
				collector := &daemonCollector{d: d}
				r.Register(collector)
				return r
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Daemon{
				ctx:    context.Background(),
				DCmds:  []*DaemonCmd{},
				Logger: slog.Default(),
			}

			err := d.RegisterMetrics(tt.registerer)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RegisterMetrics() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.expectedErr != "" && err.Error() != tt.expectedErr {
					t.Errorf("RegisterMetrics() error = %v, expectedErr %v", err.Error(), tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("RegisterMetrics() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}
