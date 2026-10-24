package natsmeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metadata/fsm"
)

const (
	StreamName = "CALLFS_META"
	Subject    = "callfs.meta.log"

	defaultApplyTimeout = 10 * time.Second
	defaultMaxMsgSize   = 8 << 20
	publishAttempts     = 3
	publishRetryDelay   = 100 * time.Millisecond
)

var ErrClosed = errors.New("natsmeta: store is closed")

type Store struct {
	nc      *nats.Conn
	js      jetstream.JetStream
	stream  jetstream.Stream
	self    string
	cfg     config.NATSMetadataConfig
	logger  *zap.Logger
	machine *fsm.Machine

	applyTimeout time.Duration
	maxMsgSize   int

	waitMu  sync.Mutex
	waiters map[string]chan fsm.Result

	commits   chan *commitItem
	groupTail []byte

	strictReads  atomic.Bool
	barrierMu    sync.Mutex
	nextRound    *barrierRound
	roundRunning bool

	appliedSeq atomic.Uint64

	progressMu sync.Mutex
	progress   chan struct{}

	haltMu sync.RWMutex
	halted error

	ctx       context.Context
	cancel    context.CancelFunc
	iter      jetstream.MessagesContext
	done      chan struct{}
	closeOnce sync.Once
}

var (
	_ metadata.Store                = (*Store)(nil)
	_ metadata.Batcher              = (*Store)(nil)
	_ metadata.ChildCounter         = (*Store)(nil)
	_ metadata.DescendantSearcher   = (*Store)(nil)
	_ metadata.ErasureMetadataStore = (*Store)(nil)
)

func Open(ctx context.Context, nc *nats.Conn, js jetstream.JetStream, self string, cfg config.NATSMetadataConfig, logger *zap.Logger) (*Store, error) {
	if nil == nc {
		return nil, errors.New("natsmeta: nats connection is required")
	}
	if nil == js {
		return nil, errors.New("natsmeta: jetstream context is required")
	}
	if "" == self {
		return nil, errors.New("natsmeta: node identity is required")
	}
	if nil == logger {
		logger = zap.NewNop()
	}
	applyTimeout := cfg.ApplyTimeout
	if 0 >= applyTimeout {
		applyTimeout = defaultApplyTimeout
	}
	maxMsgSize := int(cfg.MaxMsgSize)
	if 0 >= maxMsgSize {
		maxMsgSize = defaultMaxMsgSize
	}

	stream, err := js.Stream(ctx, StreamName)
	if nil != err {
		return nil, fmt.Errorf("natsmeta: open stream %s: %w", StreamName, err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s := &Store{
		nc:           nc,
		js:           js,
		stream:       stream,
		self:         self,
		cfg:          cfg,
		logger:       logger,
		machine:      fsm.New(),
		applyTimeout: applyTimeout,
		maxMsgSize:   maxMsgSize,
		waiters:      map[string]chan fsm.Result{},
		progress:     make(chan struct{}),
		ctx:          runCtx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}

	consumer, err := js.OrderedConsumer(ctx, StreamName, jetstream.OrderedConsumerConfig{DeliverPolicy: jetstream.DeliverAllPolicy})
	if nil != err {
		cancel()
		return nil, fmt.Errorf("natsmeta: create ordered consumer: %w", err)
	}
	iter, err := consumer.Messages(jetstream.PullMaxMessages(applierBatch))
	if nil != err {
		cancel()
		return nil, fmt.Errorf("natsmeta: start message iterator: %w", err)
	}
	s.iter = iter
	if err := s.startCommitters(); nil != err {
		cancel()
		iter.Stop()
		return nil, err
	}
	go s.runApplier()

	if err := s.catchUp(ctx); nil != err {
		return nil, errors.Join(fmt.Errorf("natsmeta: catch up with stream: %w", err), s.Close())
	}
	return s, nil
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.iter.Stop()
		<-s.done
	})
	return nil
}

func (s *Store) Ready() error {
	s.haltMu.RLock()
	defer s.haltMu.RUnlock()
	return s.halted
}

func (s *Store) LocalState() bool {
	return true
}

func (s *Store) StateHash() [32]byte {
	return s.machine.StateHash()
}

func (s *Store) StateCounts() fsm.StateCounts {
	return s.machine.StateCounts(time.Now())
}

func (s *Store) AppliedSeq() uint64 {
	return s.appliedSeq.Load()
}

func (s *Store) registerWaiter(nonce string) chan fsm.Result {
	ch := make(chan fsm.Result, 1)
	s.waitMu.Lock()
	s.waiters[nonce] = ch
	s.waitMu.Unlock()
	return ch
}

func (s *Store) dropWaiter(nonce string) {
	s.waitMu.Lock()
	delete(s.waiters, nonce)
	s.waitMu.Unlock()
}

func (s *Store) deliver(nonce string, res fsm.Result) {
	s.waitMu.Lock()
	ch, ok := s.waiters[nonce]
	s.waitMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- res:
	default:
	}
}

func (s *Store) submit(ctx context.Context, nonce string, publish func(context.Context) error) (fsm.Result, error) {
	if err := s.Ready(); nil != err {
		return fsm.Result{}, err
	}
	if nil != s.ctx.Err() {
		return fsm.Result{}, ErrClosed
	}
	waiter := s.registerWaiter(nonce)
	defer s.dropWaiter(nonce)

	callCtx, cancel := context.WithTimeout(ctx, s.applyTimeout)
	defer cancel()

	if err := publish(callCtx); nil != err {
		return fsm.Result{}, err
	}
	select {
	case res := <-waiter:
		return res, nil
	case <-callCtx.Done():
		return fsm.Result{}, fmt.Errorf("natsmeta: waiting for apply: %w", callCtx.Err())
	case <-s.ctx.Done():
		return fsm.Result{}, ErrClosed
	}
}

func (s *Store) publishWithRetry(ctx context.Context, data []byte, nonce string) error {
	var lastErr error
	for attempt := 0; publishAttempts > attempt; attempt++ {
		_, err := s.js.Publish(ctx, Subject, data, jetstream.WithMsgID(nonce))
		if nil == err {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("natsmeta: publish command: %w", errors.Join(err, ctx.Err()))
		case <-time.After(publishRetryDelay):
		}
	}
	return fmt.Errorf("natsmeta: publish command: %w", lastErr)
}

func (s *Store) apply(ctx context.Context, cmd fsm.Command) (fsm.Result, error) {
	cmd.V = fsm.Version
	cmd.Origin = s.self
	cmd.Nonce = nuid.Next()
	data, err := json.Marshal(cmd)
	if nil != err {
		return fsm.Result{}, fmt.Errorf("natsmeta: marshal command: %w", err)
	}
	res, err := s.submit(ctx, cmd.Nonce, func(callCtx context.Context) error {
		return s.enqueueCommit(callCtx, cmd.Nonce, data)
	})
	if nil != err {
		return fsm.Result{}, err
	}
	return resultError(res)
}

func resultError(res fsm.Result) (fsm.Result, error) {
	if "" != res.Err || res.Halt {
		return fsm.Result{}, res.AsError()
	}
	return res, nil
}

func (s *Store) Create(ctx context.Context, md *metadata.Metadata) error {
	if nil == md {
		return fmt.Errorf("metadata is required")
	}
	_, err := s.apply(ctx, fsm.Command{Op: "create_metadata", Metadata: fsm.CloneMetadata(md)})
	return err
}

func (s *Store) Update(ctx context.Context, md *metadata.Metadata) error {
	if nil == md {
		return fmt.Errorf("metadata is required")
	}
	_, err := s.apply(ctx, fsm.Command{Op: "update_metadata", Metadata: fsm.CloneMetadata(md)})
	return err
}

func (s *Store) Delete(ctx context.Context, path string) error {
	_, err := s.apply(ctx, fsm.Command{Op: "delete_metadata", Path: path})
	return err
}

func (s *Store) Rename(ctx context.Context, oldPath, newPath string) error {
	_, err := s.apply(ctx, fsm.Command{Op: "rename_metadata", Path: oldPath, NewPath: newPath})
	return err
}

func (s *Store) UpdateIfMatch(ctx context.Context, md *metadata.Metadata, expectedETag string) error {
	if nil == md {
		return fmt.Errorf("metadata is required")
	}
	_, err := s.apply(ctx, fsm.Command{Op: "update_metadata_if_match", Metadata: fsm.CloneMetadata(md), ExpectedETag: expectedETag})
	return err
}

func (s *Store) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	if nil == link {
		return fmt.Errorf("link is required")
	}
	if _, err := s.apply(ctx, fsm.Command{Op: "create_link", Link: fsm.CloneLink(link)}); nil != err {
		return err
	}
	link.ID = fsm.LinkIDForToken(link.Token)
	return nil
}

func (s *Store) UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error {
	_, err := s.apply(ctx, fsm.Command{Op: "update_link", Token: token, Status: status, UsedAt: usedAt, UsedByIP: usedByIP})
	return err
}

func (s *Store) ConsumeSingleUseLink(ctx context.Context, token string, usedByIP string, now time.Time) (*metadata.SingleUseLink, bool, error) {
	res, err := s.apply(ctx, fsm.Command{Op: "consume_link", Token: token, UsedAt: &now, UsedByIP: &usedByIP})
	if nil != err {
		return nil, false, err
	}
	return res.Link, res.Consumed, nil
}

func (s *Store) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	res, err := s.apply(ctx, fsm.Command{Op: "cleanup_expired_links", Before: &before})
	if nil != err {
		return 0, err
	}
	return res.CleanupCount, nil
}

func (s *Store) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	res, err := s.apply(ctx, fsm.Command{Op: "cleanup_used_links", OlderThan: &olderThan})
	if nil != err {
		return 0, err
	}
	return res.CleanupCount, nil
}

func (s *Store) RevokeSingleUseLink(ctx context.Context, id int64) error {
	_, err := s.apply(ctx, fsm.Command{Op: "revoke_link", LinkID: id})
	return err
}

func (s *Store) CreateErasureInfo(ctx context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	_, err := s.apply(ctx, fsm.Command{Op: "create_erasure_info", Path: filePath, ErasureInfo: fsm.CloneErasureFileInfo(info)})
	return err
}

func (s *Store) DeleteErasureInfo(ctx context.Context, filePath string) error {
	_, err := s.apply(ctx, fsm.Command{Op: "delete_erasure_info", Path: filePath})
	return err
}

func (s *Store) AtomicNamespace() bool {
	return true
}

const applierBatch = 4096
