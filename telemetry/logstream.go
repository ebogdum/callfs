package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ebogdum/callfs/v2/cluster"
)

const (
	logStreamLoggerName  = "log_stream"
	logStreamBuffer      = 1024
	logStreamWarnEvery   = time.Minute
	logStreamSetupWindow = 15 * time.Second
)

func isLogStreamLogger(name string) bool {
	return logStreamLoggerName == name || strings.HasSuffix(name, "."+logStreamLoggerName)
}

func StartLogStream(ctx context.Context, nc *nats.Conn, js jetstream.JetStream, logs *LogRing, minLevel string, logger *zap.Logger) (func(), error) {
	if nil == nc || nil == js || nil == logs {
		return nil, fmt.Errorf("log stream needs a NATS connection, JetStream and a log ring")
	}
	level, err := zapcore.ParseLevel(minLevel)
	if nil != err {
		return nil, fmt.Errorf("log stream level %q: %w", minLevel, err)
	}
	if nil == logger {
		logger = zap.NewNop()
	}
	logger = logger.Named(logStreamLoggerName)
	setupCtx, cancelSetup := context.WithTimeout(ctx, logStreamSetupWindow)
	defer cancelSetup()
	consumer, err := js.OrderedConsumer(setupCtx, cluster.StreamLogs, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{cluster.TelemetryWildcard(cluster.TelemetryLogs)},
		DeliverPolicy:  jetstream.DeliverNewPolicy,
	})
	if nil != err {
		return nil, fmt.Errorf("log stream consumer: %w", err)
	}
	consumed, err := consumer.Consume(func(msg jetstream.Msg) {
		var entry LogEntry
		if decodeErr := json.Unmarshal(msg.Data(), &entry); nil != decodeErr {
			logger.Debug("remote log entry unreadable", zap.String("subject", msg.Subject()), zap.Error(decodeErr))
			return
		}
		if "" == entry.Node || logs.node == entry.Node {
			return
		}
		if ingestErr := logs.Ingest(entry); nil != ingestErr {
			logger.Debug("remote log entry rejected", zap.String("node", entry.Node), zap.Error(ingestErr))
		}
	})
	if nil != err {
		return nil, fmt.Errorf("log stream consume: %w", err)
	}
	forward := &logForward{minLevel: level, out: make(chan LogEntry, logStreamBuffer)}
	logs.forward.Store(forward)
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		publishLogs(runCtx, nc, logs.node, forward, logger)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			logs.forward.CompareAndSwap(forward, nil)
			cancel()
			consumed.Stop()
			wg.Wait()
		})
	}, nil
}

func publishLogs(ctx context.Context, nc *nats.Conn, node string, forward *logForward, logger *zap.Logger) {
	var failed uint64
	var lastErr error
	lastWarn := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-forward.out:
			data, err := json.Marshal(entry)
			if nil == err {
				err = nc.Publish(cluster.TelemetrySubject(cluster.TelemetryLogs, node, entry.Level), data)
			}
			if nil != err {
				failed++
				lastErr = err
			}
		}
		dropped := forward.dropped.Swap(0)
		if (0 == failed && 0 == dropped) || logStreamWarnEvery > time.Since(lastWarn) {
			forward.dropped.Add(dropped)
			continue
		}
		logger.Warn("log entries did not reach the cluster log stream", zap.Uint64("failed", failed), zap.Uint64("dropped", dropped), zap.Error(lastErr))
		failed = 0
		lastErr = nil
		lastWarn = time.Now()
	}
}
