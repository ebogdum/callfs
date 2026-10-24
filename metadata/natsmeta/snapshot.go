package natsmeta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
)

const (
	SnapshotBucket   = "CALLFS_META_SNAP"
	SnapshotLeaseKey = "meta_snapshot"

	snapshotPrefix       = "snap-"
	snapshotMetaSeq      = "seq"
	snapshotMetaSHA256   = "sha256"
	snapshotMetaTookMS   = "took_ms"
	snapshotLeaseTTL     = 2 * time.Minute
	snapshotCheckMax     = 5 * time.Second
	snapshotReleaseLimit = 5 * time.Second
	stepDownPoll         = 100 * time.Millisecond
	defaultRetainSnaps   = 1
)

var (
	ErrSnapshotLeaseHeld   = errors.New("natsmeta: snapshot lease is held by another snapshotter")
	ErrSnapshotsDisabled   = errors.New("natsmeta: snapshot bucket " + SnapshotBucket + " is not available")
	ErrSnapshotGap         = errors.New("natsmeta: metadata log has a gap not covered by any snapshot")
	ErrNothingToSnapshot   = errors.New("natsmeta: no applied commands to snapshot")
	ErrLeaderUnchanged     = errors.New("natsmeta: metadata log leader did not change")
	errSnapshotNotDue      = errors.New("natsmeta: snapshot not due")
	errSnapshotInvalidMeta = errors.New("natsmeta: snapshot object metadata is invalid")
)

type LogInfo struct {
	FirstSeq, LastSeq, Messages, Bytes, AppliedSeq uint64
	Leader                                         string
	Replicas                                       []ReplicaInfo
	LastSnapshot                                   *SnapshotInfo
}

type ReplicaInfo struct {
	Name    string
	Current bool
	Offline bool
	Active  time.Duration
	Lag     uint64
}

type SnapshotInfo struct {
	ID        string
	Seq       uint64
	Bytes     int64
	CreatedAt time.Time
	Took      time.Duration
}

type snapshotEntry struct {
	info   SnapshotInfo
	sha256 string
}

func snapshotName(seq uint64) string {
	return snapshotPrefix + strconv.FormatUint(seq, 10)
}

func (s *Store) RetainSnapshots() int {
	if defaultRetainSnaps > s.cfg.RetainSnapshots {
		return defaultRetainSnaps
	}
	return s.cfg.RetainSnapshots
}

func (s *Store) openSnapshotBucket(ctx context.Context) error {
	obj, err := s.js.ObjectStore(ctx, SnapshotBucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) || errors.Is(err, jetstream.ErrStreamNotFound) {
		s.logger.Warn("metadata snapshot bucket missing, snapshots disabled", zap.String("bucket", SnapshotBucket))
		return nil
	}
	if nil != err {
		return fmt.Errorf("natsmeta: open snapshot bucket %s: %w", SnapshotBucket, err)
	}
	s.snapObj = obj
	return nil
}

func (s *Store) listSnapshots(ctx context.Context) ([]snapshotEntry, error) {
	if nil == s.snapObj {
		return nil, ErrSnapshotsDisabled
	}
	objects, err := s.snapObj.List(ctx)
	if errors.Is(err, jetstream.ErrNoObjectsFound) {
		return nil, nil
	}
	if nil != err {
		return nil, fmt.Errorf("natsmeta: list snapshots: %w", err)
	}
	entries := make([]snapshotEntry, 0, len(objects))
	for _, object := range objects {
		if nil == object || object.Deleted || !strings.HasPrefix(object.Name, snapshotPrefix) {
			continue
		}
		entry, err := entryFromObject(object)
		if nil != err {
			s.logger.Warn("metadata snapshot skipped", zap.String("id", object.Name), zap.Error(err))
			continue
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b snapshotEntry) int {
		if a.info.Seq > b.info.Seq {
			return -1
		}
		if a.info.Seq < b.info.Seq {
			return 1
		}
		return 0
	})
	return entries, nil
}

func entryFromObject(object *jetstream.ObjectInfo) (snapshotEntry, error) {
	seq, err := strconv.ParseUint(object.Metadata[snapshotMetaSeq], 10, 64)
	if nil != err {
		return snapshotEntry{}, fmt.Errorf("%w: seq: %w", errSnapshotInvalidMeta, err)
	}
	if snapshotName(seq) != object.Name {
		return snapshotEntry{}, fmt.Errorf("%w: name %s does not match seq %d", errSnapshotInvalidMeta, object.Name, seq)
	}
	sum := object.Metadata[snapshotMetaSHA256]
	if sha256.Size*2 != len(sum) {
		return snapshotEntry{}, fmt.Errorf("%w: sha256 %q", errSnapshotInvalidMeta, sum)
	}
	var took time.Duration
	if raw, ok := object.Metadata[snapshotMetaTookMS]; ok {
		ms, err := strconv.ParseInt(raw, 10, 64)
		if nil != err {
			return snapshotEntry{}, fmt.Errorf("%w: took_ms: %w", errSnapshotInvalidMeta, err)
		}
		took = time.Duration(ms) * time.Millisecond
	}
	return snapshotEntry{
		info: SnapshotInfo{
			ID:        object.Name,
			Seq:       seq,
			Bytes:     int64(object.Size),
			CreatedAt: object.ModTime,
			Took:      took,
		},
		sha256: sum,
	}, nil
}

func (s *Store) Snapshots(ctx context.Context) ([]SnapshotInfo, error) {
	entries, err := s.listSnapshots(ctx)
	if nil != err {
		return nil, err
	}
	out := make([]SnapshotInfo, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.info)
	}
	return out, nil
}

func (s *Store) latestSnapshot(ctx context.Context) (*SnapshotInfo, error) {
	entries, err := s.listSnapshots(ctx)
	if nil != err {
		return nil, err
	}
	if 0 == len(entries) {
		return nil, nil
	}
	latest := entries[0].info
	return &latest, nil
}

func (s *Store) restoreFromSnapshots(ctx context.Context, firstSeq, lastSeq uint64) (uint64, error) {
	var entries []snapshotEntry
	if nil != s.snapObj {
		listed, err := s.listSnapshots(ctx)
		if nil != err {
			return 0, err
		}
		entries = listed
	}
	for _, entry := range entries {
		if lastSeq < entry.info.Seq || firstSeq > entry.info.Seq+1 {
			s.logger.Warn("metadata snapshot outside stream range",
				zap.String("id", entry.info.ID), zap.Uint64("first_seq", firstSeq), zap.Uint64("last_seq", lastSeq))
			continue
		}
		if err := s.restoreSnapshot(ctx, entry); nil != err {
			s.logger.Warn("metadata snapshot unusable", zap.String("id", entry.info.ID), zap.Error(err))
			continue
		}
		s.appliedSeq.Store(entry.info.Seq)
		s.rememberSnapshot(entry.info)
		s.logger.Info("metadata restored from snapshot", zap.String("id", entry.info.ID), zap.Uint64("seq", entry.info.Seq))
		return entry.info.Seq, nil
	}
	if 0 == lastSeq || 1 >= firstSeq {
		return 0, nil
	}
	return 0, fmt.Errorf("%w: stream %s starts at sequence %d", ErrSnapshotGap, StreamName, firstSeq)
}

func (s *Store) restoreSnapshot(ctx context.Context, entry snapshotEntry) error {
	data, err := s.snapObj.GetBytes(ctx, entry.info.ID)
	if nil != err {
		return fmt.Errorf("read snapshot: %w", err)
	}
	sum := sha256.Sum256(data)
	if entry.sha256 != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("sha256 mismatch for %s", entry.info.ID)
	}
	return s.machine.Restore(bytes.NewReader(data))
}

func (s *Store) rememberSnapshot(info SnapshotInfo) {
	s.snapStateMu.Lock()
	if info.Seq >= s.lastSnapSeq {
		s.lastSnapSeq = info.Seq
		s.lastSnapAt = info.CreatedAt
	}
	s.snapStateMu.Unlock()
}

func (s *Store) snapshotDue(lastSeq uint64, lastAt time.Time, now time.Time) bool {
	applied := s.appliedSeq.Load()
	if applied <= lastSeq {
		return false
	}
	if 0 < s.cfg.SnapshotEvery && applied-lastSeq >= s.cfg.SnapshotEvery {
		return true
	}
	return 0 < s.cfg.SnapshotInterval && now.Sub(lastAt) >= s.cfg.SnapshotInterval
}

func (s *Store) cachedSnapshotDue(now time.Time) bool {
	s.snapStateMu.Lock()
	lastSeq, lastAt := s.lastSnapSeq, s.lastSnapAt
	s.snapStateMu.Unlock()
	return s.snapshotDue(lastSeq, lastAt, now)
}

func (s *Store) TakeSnapshot(ctx context.Context) (SnapshotInfo, error) {
	return s.takeSnapshot(ctx, true)
}

func (s *Store) takeSnapshot(ctx context.Context, force bool) (SnapshotInfo, error) {
	if nil == s.snapObj {
		return SnapshotInfo{}, ErrSnapshotsDisabled
	}
	if err := s.Ready(); nil != err {
		return SnapshotInfo{}, err
	}
	if nil != s.ctx.Err() {
		return SnapshotInfo{}, ErrClosed
	}
	s.snapMu.Lock()
	defer s.snapMu.Unlock()

	release, err := s.acquireSnapshotLease(ctx)
	if nil != err {
		return SnapshotInfo{}, err
	}
	defer release()

	latest, err := s.latestSnapshot(ctx)
	if nil != err {
		return SnapshotInfo{}, err
	}
	if nil != latest {
		s.rememberSnapshot(*latest)
	}
	if !force {
		lastSeq, lastAt := uint64(0), s.openedAt
		if nil != latest {
			lastSeq, lastAt = latest.Seq, latest.CreatedAt
		}
		if !s.snapshotDue(lastSeq, lastAt, time.Now()) {
			return SnapshotInfo{}, errSnapshotNotDue
		}
	}

	started := time.Now()
	s.applyMu.Lock()
	captured := s.machine.Capture()
	seq := s.appliedSeq.Load()
	s.applyMu.Unlock()
	if 0 == seq {
		return SnapshotInfo{}, ErrNothingToSnapshot
	}
	if nil != latest && latest.Seq >= seq {
		return *latest, nil
	}

	var buf bytes.Buffer
	sum, err := captured.Write(&buf)
	if nil != err {
		return SnapshotInfo{}, fmt.Errorf("natsmeta: encode snapshot at sequence %d: %w", seq, err)
	}
	took := time.Since(started)
	name := snapshotName(seq)
	digest := hex.EncodeToString(sum[:])
	stored, err := s.snapObj.Put(ctx, jetstream.ObjectMeta{
		Name: name,
		Headers: map[string][]string{
			"Callfs-Snapshot-Seq":    {strconv.FormatUint(seq, 10)},
			"Callfs-Snapshot-Sha256": {digest},
		},
		Metadata: map[string]string{
			snapshotMetaSeq:    strconv.FormatUint(seq, 10),
			snapshotMetaSHA256: digest,
			snapshotMetaTookMS: strconv.FormatInt(took.Milliseconds(), 10),
		},
	}, bytes.NewReader(buf.Bytes()))
	if nil != err {
		return SnapshotInfo{}, fmt.Errorf("natsmeta: store snapshot %s: %w", name, err)
	}
	info := SnapshotInfo{ID: name, Seq: seq, Bytes: int64(stored.Size), CreatedAt: stored.ModTime, Took: took}
	s.rememberSnapshot(info)

	floor, purgeErr := s.purgeBelow(ctx, seq)
	pruned, pruneErr := s.pruneSnapshots(ctx)
	s.logger.Info("metadata snapshot stored",
		zap.String("id", name), zap.Uint64("seq", seq), zap.Int64("bytes", info.Bytes),
		zap.Duration("took", took), zap.Uint64("purge_floor", floor), zap.Int("pruned", pruned))
	if err := errors.Join(purgeErr, pruneErr); nil != err {
		return info, err
	}
	return info, nil
}

func (s *Store) acquireSnapshotLease(ctx context.Context) (func(), error) {
	kv, err := s.js.KeyValue(ctx, cluster.BucketLeases)
	if nil != err {
		return nil, fmt.Errorf("natsmeta: open lease bucket %s: %w", cluster.BucketLeases, err)
	}
	rev, err := kv.Create(ctx, SnapshotLeaseKey, []byte(s.self), jetstream.KeyTTL(snapshotLeaseTTL))
	if errors.Is(err, jetstream.ErrKeyExists) {
		holder := "unknown"
		if entry, getErr := kv.Get(ctx, SnapshotLeaseKey); nil == getErr && nil != entry {
			holder = string(entry.Value())
		}
		return nil, fmt.Errorf("%w: holder %s", ErrSnapshotLeaseHeld, holder)
	}
	if nil != err {
		return nil, fmt.Errorf("natsmeta: acquire snapshot lease: %w", err)
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), snapshotReleaseLimit)
		defer cancel()
		if err := kv.Delete(releaseCtx, SnapshotLeaseKey, jetstream.LastRevision(rev)); nil != err {
			s.logger.Warn("metadata snapshot lease release failed", zap.Error(err))
		}
	}, nil
}

func (s *Store) purgeBelow(ctx context.Context, seq uint64) (uint64, error) {
	floor := uint64(1)
	if seq > s.cfg.RetainCommands {
		floor = seq - s.cfg.RetainCommands
	}
	if seq+1 < floor {
		floor = seq + 1
	}
	if 1 >= floor {
		return 1, nil
	}
	if err := s.stream.Purge(ctx, jetstream.WithPurgeSequence(floor)); nil != err {
		return floor, fmt.Errorf("natsmeta: purge stream %s below sequence %d: %w", StreamName, floor, err)
	}
	return floor, nil
}

func (s *Store) pruneSnapshots(ctx context.Context) (int, error) {
	entries, err := s.listSnapshots(ctx)
	if nil != err {
		return 0, err
	}
	retain := s.RetainSnapshots()
	if len(entries) <= retain {
		return 0, nil
	}
	var failures []error
	pruned := 0
	for _, entry := range entries[retain:] {
		if err := s.snapObj.Delete(ctx, entry.info.ID); nil != err {
			failures = append(failures, fmt.Errorf("natsmeta: delete snapshot %s: %w", entry.info.ID, err))
			continue
		}
		pruned++
	}
	return pruned, errors.Join(failures...)
}

func (s *Store) snapshotCheckInterval() time.Duration {
	if 0 < s.cfg.SnapshotInterval && snapshotCheckMax > s.cfg.SnapshotInterval {
		return s.cfg.SnapshotInterval
	}
	return snapshotCheckMax
}

func (s *Store) runSnapshotter() {
	defer close(s.snapDone)
	if nil == s.snapObj || (0 >= s.cfg.SnapshotInterval && 0 == s.cfg.SnapshotEvery) {
		return
	}
	ticker := time.NewTicker(s.snapshotCheckInterval())
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			if !s.cachedSnapshotDue(now) {
				continue
			}
			if nil != s.Ready() {
				continue
			}
			_, err := s.takeSnapshot(s.ctx, false)
			if nil == err || errors.Is(err, errSnapshotNotDue) || errors.Is(err, ErrSnapshotLeaseHeld) || nil != s.ctx.Err() {
				continue
			}
			s.logger.Warn("scheduled metadata snapshot failed", zap.Error(err))
		}
	}
}

func (s *Store) LogInfo(ctx context.Context) (LogInfo, error) {
	info, err := s.stream.Info(ctx)
	if nil != err {
		return LogInfo{}, fmt.Errorf("natsmeta: read stream info: %w", err)
	}
	out := LogInfo{
		FirstSeq:   info.State.FirstSeq,
		LastSeq:    info.State.LastSeq,
		Messages:   info.State.Msgs,
		Bytes:      info.State.Bytes,
		AppliedSeq: s.appliedSeq.Load(),
	}
	if nil != info.Cluster {
		out.Leader = info.Cluster.Leader
		if "" != info.Cluster.Leader {
			out.Replicas = append(out.Replicas, ReplicaInfo{Name: info.Cluster.Leader, Current: true})
		}
		for _, peer := range info.Cluster.Replicas {
			if nil == peer {
				continue
			}
			out.Replicas = append(out.Replicas, ReplicaInfo{
				Name:    peer.Name,
				Current: peer.Current,
				Offline: peer.Offline,
				Active:  peer.Active,
				Lag:     peer.Lag,
			})
		}
	}
	if nil != s.snapObj {
		latest, err := s.latestSnapshot(ctx)
		if nil != err {
			return LogInfo{}, err
		}
		out.LastSnapshot = latest
	}
	return out, nil
}

type stepDownPlacement struct {
	Preferred string `json:"preferred,omitempty"`
}

type stepDownRequest struct {
	Placement *stepDownPlacement `json:"placement,omitempty"`
}

type stepDownResponse struct {
	Error   *jetstream.APIError `json:"error,omitempty"`
	Success bool                `json:"success,omitempty"`
}

func (s *Store) streamLeader(ctx context.Context) (string, error) {
	info, err := s.stream.Info(ctx)
	if nil != err {
		return "", fmt.Errorf("natsmeta: read stream info: %w", err)
	}
	if nil == info.Cluster {
		return "", nil
	}
	return info.Cluster.Leader, nil
}

func (s *Store) StepDown(ctx context.Context, preferred string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.applyTimeout)
	defer cancel()

	previous, err := s.streamLeader(callCtx)
	if nil != err {
		return "", err
	}
	var body []byte
	if "" != preferred {
		body, err = json.Marshal(stepDownRequest{Placement: &stepDownPlacement{Preferred: preferred}})
		if nil != err {
			return "", fmt.Errorf("natsmeta: encode stepdown request: %w", err)
		}
	}
	reply, err := s.nc.RequestWithContext(callCtx, "$JS.API.STREAM.LEADER.STEPDOWN."+StreamName, body)
	if nil != err {
		return "", fmt.Errorf("natsmeta: request leader stepdown: %w", err)
	}
	var resp stepDownResponse
	if err := json.Unmarshal(reply.Data, &resp); nil != err {
		return "", fmt.Errorf("natsmeta: decode stepdown response: %w", err)
	}
	if nil != resp.Error {
		return "", fmt.Errorf("natsmeta: leader stepdown: %w", resp.Error)
	}
	if !resp.Success {
		return "", errors.New("natsmeta: leader stepdown was not accepted")
	}

	ticker := time.NewTicker(stepDownPoll)
	defer ticker.Stop()
	for {
		leader, err := s.streamLeader(callCtx)
		if nil == err && "" != leader && previous != leader {
			s.logger.Info("metadata log leader changed", zap.String("previous", previous), zap.String("leader", leader))
			return leader, nil
		}
		select {
		case <-callCtx.Done():
			return "", fmt.Errorf("%w from %s: %w", ErrLeaderUnchanged, previous, errors.Join(err, callCtx.Err()))
		case <-ticker.C:
		}
	}
}
