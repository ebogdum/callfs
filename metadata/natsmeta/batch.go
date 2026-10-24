package natsmeta

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nuid"

	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metadata/fsm"
	"github.com/ebogdum/callfs/v2/natsrv"
)

const (
	envelopeMargin = 4096
	partOverhead   = 512
)

var batchOpNames = map[metadata.OpKind]string{
	metadata.OpCreate:            "create_metadata",
	metadata.OpUpdate:            "update_metadata",
	metadata.OpUpdateIfMatch:     "update_metadata_if_match",
	metadata.OpDelete:            "delete_metadata",
	metadata.OpRename:            "rename_metadata",
	metadata.OpCreateErasureInfo: "create_erasure_info",
	metadata.OpDeleteErasureInfo: "delete_erasure_info",
}

func (s *Store) ApplyBatch(ctx context.Context, ops []metadata.Op) error {
	if 0 == len(ops) {
		return nil
	}
	if err := metadata.ValidateOps(ops); nil != err {
		return err
	}
	cmds := make([]fsm.Command, 0, len(ops))
	for _, op := range ops {
		name, ok := batchOpNames[op.Kind]
		if !ok {
			return fmt.Errorf("%w: unknown kind %q", metadata.ErrInvalidOp, op.Kind)
		}
		cmds = append(cmds, fsm.Command{
			Op:           name,
			Path:         op.Path,
			NewPath:      op.NewPath,
			Metadata:     fsm.CloneMetadata(op.Metadata),
			ExpectedETag: op.ExpectedETag,
			ErasureInfo:  fsm.CloneErasureFileInfo(op.ErasureInfo),
		})
	}

	single := fsm.Command{Op: "apply_batch", Ops: cmds, V: fsm.Version, Origin: s.self, Nonce: nuid.Next()}
	data, err := json.Marshal(single)
	if nil != err {
		return fmt.Errorf("natsmeta: marshal batch: %w", err)
	}
	limit := s.maxMsgSize - envelopeMargin
	if len(data) <= limit {
		res, err := s.submit(ctx, single.Nonce, func(callCtx context.Context) error {
			return s.publishWithRetry(callCtx, data, single.Nonce)
		})
		if nil != err {
			return err
		}
		_, err = resultError(res)
		return err
	}
	return s.applySplitBatch(ctx, cmds, limit)
}

func (s *Store) applySplitBatch(ctx context.Context, cmds []fsm.Command, limit int) error {
	chunks, err := splitOps(cmds, limit)
	if nil != err {
		return err
	}
	if natsrv.MaxAtomicBatch < len(chunks) {
		return fmt.Errorf("%w: batch needs %d parts, limit is %d", metadata.ErrInvalidOp, len(chunks), natsrv.MaxAtomicBatch)
	}

	batchID := nuid.Next()
	nonce := nuid.Next()
	payloads := make([][]byte, 0, len(chunks))
	last := len(chunks) - 1
	for i, chunk := range chunks {
		part := fsm.Command{Op: "batch_part", Ops: chunk, V: fsm.Version, BatchID: batchID, BatchSeq: i}
		if last == i {
			part.BatchCommit = true
			part.Origin = s.self
			part.Nonce = nonce
		}
		data, err := json.Marshal(part)
		if nil != err {
			return fmt.Errorf("natsmeta: marshal batch part %d: %w", i, err)
		}
		payloads = append(payloads, data)
	}

	res, err := s.submit(ctx, nonce, func(callCtx context.Context) error {
		if _, err := natsrv.PublishAtomic(callCtx, s.nc, Subject, payloads); nil != err {
			return fmt.Errorf("natsmeta: publish batch: %w", err)
		}
		return nil
	})
	if nil != err {
		return err
	}
	_, err = resultError(res)
	return err
}

func splitOps(cmds []fsm.Command, limit int) ([][]fsm.Command, error) {
	budget := limit - partOverhead
	chunks := make([][]fsm.Command, 0)
	current := make([]fsm.Command, 0)
	size := 0
	for i := range cmds {
		encoded, err := json.Marshal(cmds[i])
		if nil != err {
			return nil, fmt.Errorf("natsmeta: marshal batch op %d: %w", i, err)
		}
		opSize := len(encoded) + 1
		if opSize > budget {
			return nil, fmt.Errorf("%w: operation %d is %d bytes, part limit is %d", metadata.ErrInvalidOp, i, opSize, budget)
		}
		if 0 < len(current) && size+opSize > budget {
			chunks = append(chunks, current)
			current = make([]fsm.Command, 0)
			size = 0
		}
		current = append(current, cmds[i])
		size += opSize
	}
	if 0 < len(current) {
		chunks = append(chunks, current)
	}
	return chunks, nil
}
