package config

import (
	"log/slog"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Coordinator interface {
	Subscribe(...func(*Conf) error)
	Notify() error
	Reload() error
}

type coordinator struct {
	configFile string
	config     *Conf
	logger     *slog.Logger

	mu          sync.Mutex
	subscribers []func(*Conf) error

	configSuccessMetric     prometheus.Gauge
	configSuccessTimeMetric prometheus.Gauge
}

var _ Coordinator = (*coordinator)(nil)

func NewCoordinator(logger *slog.Logger, configFile string, r prometheus.Registerer) Coordinator {
	c := &coordinator{
		configFile: configFile,
		logger:     logger,
	}

	c.registerMetrics(r)

	return c
}

func (c *coordinator) Subscribe(ss ...func(*Conf) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscribers = append(c.subscribers, ss...)
}

func (c *coordinator) Notify() error {
	for _, s := range c.subscribers {
		if err := s(c.config); err != nil {
			c.logger.Error("notify subscriber failed", "err", err)
			return err
		}
	}
	return nil
}

func (c *coordinator) Reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("reload config file", "file", c.configFile)
	if err := c.loadconfig(); err != nil {
		c.logger.Error("load config file failed", "file", c.configFile, "err", err)
		c.configSuccessMetric.Set(0)
		return err
	}

	if err := c.Notify(); err != nil {
		c.logger.Error("notify subscriber failed", "err", err)
		c.configSuccessMetric.Set(0)
	}

	c.configSuccessMetric.Set(1)
	c.configSuccessTimeMetric.SetToCurrentTime()

	return nil

}

func (c *coordinator) loadconfig() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	bys, err := os.ReadFile(c.configFile)
	if err != nil {
		c.logger.Error("read config file failed", "file", c.configFile, "err", err)
		return err
	}
	conf, err := Unmarshal(bys)
	if err != nil {
		c.logger.Error("unmarshal config file failed", "file", c.configFile, "err", err)
		return err
	}
	c.config = conf
	return nil
}

func (c *coordinator) registerMetrics(r prometheus.Registerer) {
	configSuccess := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "daemon_config_last_reload_successful",
		Help: "Whether the last configuration reload attempt was successful.",
	})
	configSuccessTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "daemon_config_last_reload_success_timestamp_seconds",
		Help: "Timestamp of the last successful configuration reload.",
	})

	r.MustRegister(configSuccess, configSuccessTime)

	c.configSuccessMetric = configSuccess
	c.configSuccessTimeMetric = configSuccessTime
}
