package daemon

import "github.com/prometheus/client_golang/prometheus"

var (
	// 1 = running, 0 = stopped
	dcmdStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "daemon_cmd_status",
			Help: "Status of daemon cmd",
		},
		[]string{"name", "port", "hostname", "ip"},
	)

	dcmdRestartCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "daemon_cmd_restart_total",
			Help: "Total number of restarts for each daemon cmd",
		},
		[]string{"name", "port", "hostname", "ip"},
	)
)

type daemonCollector struct {
	d *Daemon
}

var _ prometheus.Collector = (*daemonCollector)(nil)

func (collector *daemonCollector) Describe(ch chan<- *prometheus.Desc) {
	dcmdStatus.Describe(ch)
	dcmdRestartCount.Describe(ch)
}

func (collector *daemonCollector) Collect(ch chan<- prometheus.Metric) {
	for _, dcmd := range collector.d.DCmds {
		dcmdStatus.WithLabelValues(
			dcmd.Annotations[AnnotationsNameKey],
			dcmd.Annotations[AnnotationsPortKey],
			dcmd.Annotations[AnnotationsHostnameKey],
			dcmd.Annotations[AnnotationsIPKey],
		).Set(float64(dcmd.Status))
	}
	dcmdStatus.Collect(ch)
	dcmdRestartCount.Collect(ch)
}
