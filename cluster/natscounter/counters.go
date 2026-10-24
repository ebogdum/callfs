package natscounter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	StreamName = "CALLFS_COUNTERS"

	subjectPrefix    = "callfs.counters."
	incrHeader       = "Nats-Incr"
	streamMaxAge     = 2 * time.Hour
	heldRefreshEvery = streamMaxAge / 4
	minWindowSeconds = 1
	maxWindowSeconds = 60
	defaultTimeout   = 2 * time.Second
	failureLogEvery  = time.Minute
	keyHashBytes     = 16
	slotScopeSubject = "slot"
	rateScopeSubject = "rate"
)

var ErrInvalidCounterValue = errors.New("counter ack carried no valid value")

type Options struct {
	Replicas    int
	ClusterSize int
	Timeout     time.Duration
	Logger      *zap.Logger
}

type Counters struct {
	js       jetstream.JetStream
	replicas int
	timeout  time.Duration
	logger   *zap.Logger
	now      func() time.Time

	mu    sync.Mutex
	ready bool

	heldMu     sync.Mutex
	held       map[string]int
	refreshing bool

	lastFailureLog atomic.Int64
}

func New(nc *nats.Conn, opts Options) (*Counters, error) {
	if nil == nc {
		return nil, errors.New("nats connection cannot be nil")
	}
	js, err := jetstream.New(nc)
	if nil != err {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}
	logger := opts.Logger
	if nil == logger {
		logger = zap.NewNop()
	}
	timeout := opts.Timeout
	if 0 >= timeout {
		timeout = defaultTimeout
	}
	return &Counters{
		js:       js,
		replicas: cappedReplicas(opts.Replicas, opts.ClusterSize),
		timeout:  timeout,
		logger:   logger,
		now:      time.Now,
		held:     map[string]int{},
	}, nil
}

func cappedReplicas(requested, clusterSize int) int {
	replicas := requested
	if 0 < clusterSize && clusterSize < replicas {
		replicas = clusterSize
	}
	if 1 > replicas {
		return 1
	}
	return replicas
}

func (c *Counters) ensureStream(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready {
		return nil
	}
	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:              StreamName,
		Subjects:          []string{subjectPrefix + ">"},
		Storage:           jetstream.MemoryStorage,
		Replicas:          c.replicas,
		MaxAge:            streamMaxAge,
		MaxMsgsPerSubject: 1,
		AllowMsgCounter:   true,
	})
	if nil != err {
		return fmt.Errorf("failed to ensure counter stream: %w", err)
	}
	c.ready = true
	return nil
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:keyHashBytes])
}

func (c *Counters) add(ctx context.Context, subject string, delta int64) (int64, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.ensureStream(callCtx); nil != err {
		return 0, err
	}
	msg := nats.NewMsg(subject)
	msg.Header.Set(incrHeader, strconv.FormatInt(delta, 10))
	ack, err := c.js.PublishMsg(callCtx, msg)
	if nil != err {
		return 0, fmt.Errorf("failed to publish counter increment: %w", err)
	}
	if "" == ack.Value {
		return 0, ErrInvalidCounterValue
	}
	value, err := strconv.ParseInt(ack.Value, 10, 64)
	if nil != err {
		return 0, fmt.Errorf("%w: %w", ErrInvalidCounterValue, err)
	}
	return value, nil
}

func windowSeconds(rps float64, burst int) int64 {
	if !(0 < rps) {
		return maxWindowSeconds
	}
	seconds := int64(math.Ceil(float64(burst) / rps))
	return min(max(seconds, minWindowSeconds), maxWindowSeconds)
}

func windowAllowance(rps float64, burst int, seconds int64) int64 {
	return max(int64(burst), int64(math.Ceil(rps*float64(seconds))))
}

func (c *Counters) Allow(ctx context.Context, endpoint, client string, rps float64, burst int) (bool, error) {
	seconds := windowSeconds(rps, burst)
	window := c.now().Unix() / seconds
	subject := subjectPrefix + rateScopeSubject + "." + endpoint + "." + strconv.FormatInt(window, 10) + "." + hashKey(client)
	value, err := c.add(ctx, subject, 1)
	if nil != err {
		return false, err
	}
	return value <= windowAllowance(rps, burst, seconds), nil
}

func (c *Counters) Acquire(ctx context.Context, scope, key string, limit int) (bool, error) {
	subject := subjectPrefix + slotScopeSubject + "." + scope + "." + hashKey(key)
	value, err := c.add(ctx, subject, 1)
	if nil != err {
		return false, err
	}
	if value <= int64(limit) {
		c.markHeld(subject, 1)
		return true, nil
	}
	if _, undoErr := c.add(ctx, subject, -1); nil != undoErr {
		return false, undoErr
	}
	return false, nil
}

func (c *Counters) Release(ctx context.Context, scope, key string) error {
	subject := subjectPrefix + slotScopeSubject + "." + scope + "." + hashKey(key)
	c.markHeld(subject, -1)
	value, err := c.add(ctx, subject, -1)
	if nil != err {
		return err
	}
	if 0 > value {
		_, err = c.add(ctx, subject, -value)
	}
	return err
}

func (c *Counters) markHeld(subject string, delta int) {
	c.heldMu.Lock()
	defer c.heldMu.Unlock()
	count := c.held[subject] + delta
	if 0 < count {
		c.held[subject] = count
	} else {
		delete(c.held, subject)
	}
	if 0 < len(c.held) && !c.refreshing {
		c.refreshing = true
		go c.refreshHeld()
	}
}

func (c *Counters) heldSubjects() []string {
	c.heldMu.Lock()
	defer c.heldMu.Unlock()
	if 0 == len(c.held) {
		c.refreshing = false
		return nil
	}
	subjects := make([]string, 0, len(c.held))
	for subject := range c.held {
		subjects = append(subjects, subject)
	}
	return subjects
}

func (c *Counters) refreshHeld() {
	ticker := time.NewTicker(heldRefreshEvery)
	defer ticker.Stop()
	for range ticker.C {
		subjects := c.heldSubjects()
		if nil == subjects {
			return
		}
		for _, subject := range subjects {
			if _, err := c.add(context.Background(), subject, 0); nil != err {
				c.ReportFailure("refresh", err)
			}
		}
	}
}

func (c *Counters) ReportFailure(op string, err error) {
	now := c.now().UnixNano()
	last := c.lastFailureLog.Load()
	if 0 != last && now-last < int64(failureLogEvery) {
		return
	}
	if !c.lastFailureLog.CompareAndSwap(last, now) {
		return
	}
	c.logger.Warn("Cluster counters unavailable, using local limits", zap.String("op", op), zap.Error(err))
}
