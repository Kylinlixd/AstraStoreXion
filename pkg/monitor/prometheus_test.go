package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrometheusMonitorUpdatesUnlabelledMetrics(t *testing.T) {
	mon := NewPrometheusMonitor(MonitorConfig{})

	require.NoError(t, mon.RegisterMetric(MetricDefinition{
		Name: "xion_test_total",
		Help: "test counter",
		Type: CounterMetric,
	}))
	require.NoError(t, mon.RegisterMetric(MetricDefinition{
		Name: "xion_test_gauge",
		Help: "test gauge",
		Type: GaugeMetric,
	}))
	require.NoError(t, mon.RegisterMetric(MetricDefinition{
		Name: "xion_test_seconds",
		Help: "test histogram",
		Type: HistogramMetric,
	}))

	require.NoError(t, mon.IncCounter("xion_test_total", 1, nil))
	require.NoError(t, mon.SetGauge("xion_test_gauge", 2, nil))
	require.NoError(t, mon.Observe("xion_test_seconds", 0.25, nil))
}
