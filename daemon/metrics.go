package daemon

import "github.com/prometheus/client_golang/prometheus"

var (
	// 1 = running, 0 = stopped
	dcmdStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "daemon_cmd_status",
			Help: "Status of daemon cmd",
		},
		[]string{"name", "port", "status", "err", "hostname", "admIP"},
	)
)


