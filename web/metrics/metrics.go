package metrics

import (
	"cmdDaemon/daemon"

	"github.com/go-kit/kit/metrics"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type DaemonMetrics struct {
	d              *daemon.Daemon
	CmdStatusTotal metrics.Gauge
}

func NewDaemonMetrics(d *daemon.Daemon) *DaemonMetrics {
	CmdStatusTotal := kitprometheus.NewGaugeFrom(
		stdprometheus.GaugeOpts{
			Name: "daemon_cmd_total",
			Help: "Number of daemon cmds",
		}, []string{"status"},
	)

	return &DaemonMetrics{
		d:              d,
		CmdStatusTotal: CmdStatusTotal,
	}
}
