package natsrv

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestCollectorNilServer(t *testing.T) {
	collector := NewCollector(nil)

	descs := make(chan *prometheus.Desc, 16)
	collector.Describe(descs)
	if 0 == len(descs) {
		t.Fatal("Describe sent no descriptors")
	}

	metrics := make(chan prometheus.Metric, 16)
	collector.Collect(metrics)
	if 0 != len(metrics) {
		t.Fatal("Collect with nil server sent metrics")
	}
}
