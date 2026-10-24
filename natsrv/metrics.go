package natsrv

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	refreshInterval = 2 * time.Second

	connectedMetric      = "callfs_nats_connected"
	routesMetric         = "callfs_nats_routes"
	slowConsumersMetric  = "callfs_nats_slow_consumers"
	inMsgsMetric         = "callfs_nats_in_msgs"
	outBytesMetric       = "callfs_nats_out_bytes"
	jsAPIErrorsMetric    = "callfs_nats_js_api_errors"
	streamMessagesMetric = "callfs_nats_stream_messages"
	streamBytesMetric    = "callfs_nats_stream_bytes"
)

type Collector struct {
	ns    *server.Server
	mu    sync.Mutex
	at    time.Time
	varz  *server.Varz
	jsz   *server.JSInfo
	descs map[string]*prometheus.Desc
}

func NewCollector(ns *server.Server) *Collector {
	descs := map[string]*prometheus.Desc{
		connectedMetric:      prometheus.NewDesc(connectedMetric, "1 if the embedded NATS server is running, 0 otherwise.", nil, nil),
		routesMetric:         prometheus.NewDesc(routesMetric, "Number of connected NATS cluster routes.", nil, nil),
		slowConsumersMetric:  prometheus.NewDesc(slowConsumersMetric, "Total clients disconnected as slow consumers since start.", nil, nil),
		inMsgsMetric:         prometheus.NewDesc(inMsgsMetric, "Total messages received by the NATS server.", nil, nil),
		outBytesMetric:       prometheus.NewDesc(outBytesMetric, "Total bytes sent by the NATS server.", nil, nil),
		jsAPIErrorsMetric:    prometheus.NewDesc(jsAPIErrorsMetric, "Total JetStream API requests that resulted in an error.", nil, nil),
		streamMessagesMetric: prometheus.NewDesc(streamMessagesMetric, "Number of messages stored in a JetStream stream.", []string{"stream"}, nil),
		streamBytesMetric:    prometheus.NewDesc(streamBytesMetric, "Number of bytes stored in a JetStream stream.", []string{"stream"}, nil),
	}
	return &Collector{ns: ns, descs: descs}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if nil == c.ns {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	var refreshErr error
	if refreshInterval < time.Since(c.at) {
		refreshErr = c.refresh()
	}

	c.send(ch, connectedMetric, prometheus.GaugeValue, boolGauge(c.ns.Running()))
	if nil != refreshErr {
		ch <- prometheus.NewInvalidMetric(c.descs[connectedMetric], refreshErr)
	}

	if nil != c.varz {
		c.send(ch, routesMetric, prometheus.GaugeValue, float64(c.varz.Routes))
		c.send(ch, slowConsumersMetric, prometheus.CounterValue, float64(c.varz.SlowConsumers))
		c.send(ch, inMsgsMetric, prometheus.CounterValue, float64(c.varz.InMsgs))
		c.send(ch, outBytesMetric, prometheus.CounterValue, float64(c.varz.OutBytes))
	}

	if nil != c.jsz {
		c.send(ch, jsAPIErrorsMetric, prometheus.CounterValue, float64(c.jsz.API.Errors))
		for _, account := range c.jsz.AccountDetails {
			for _, stream := range account.Streams {
				c.send(ch, streamMessagesMetric, prometheus.GaugeValue, float64(stream.State.Msgs), stream.Name)
				c.send(ch, streamBytesMetric, prometheus.GaugeValue, float64(stream.State.Bytes), stream.Name)
			}
		}
	}
}

func (c *Collector) refresh() error {
	c.at = time.Now()
	varz, varzErr := c.ns.Varz(&server.VarzOptions{})
	if nil == varzErr {
		c.varz = varz
	}
	jsz, jszErr := c.ns.Jsz(&server.JSzOptions{Streams: true})
	if nil == jszErr {
		c.jsz = jsz
	}
	if nil != varzErr {
		return fmt.Errorf("read varz: %w", varzErr)
	}
	if nil != jszErr {
		return fmt.Errorf("read jsz: %w", jszErr)
	}
	return nil
}

func (c *Collector) send(ch chan<- prometheus.Metric, name string, valueType prometheus.ValueType, value float64, labelValues ...string) {
	metric, err := prometheus.NewConstMetric(c.descs[name], valueType, value, labelValues...)
	if nil != err {
		ch <- prometheus.NewInvalidMetric(c.descs[name], err)
		return
	}
	ch <- metric
}

func boolGauge(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
