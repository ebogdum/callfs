package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

var _ metadata.Batcher = (*PostgresStore)(nil)

type dbQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const pingTimeout = 10 * time.Second

// PostgresStore implements the metadata.Store interface using PostgreSQL
type PostgresStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewPostgresStore creates a new PostgreSQL metadata store
func NewPostgresStore(dsn string, logger *zap.Logger) (*PostgresStore, error) {
	return NewPostgresStoreWithPool(dsn, 25, 25, 5*time.Minute, logger)
}

func NewPostgresStoreWithPool(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetime time.Duration, logger *zap.Logger) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	// Test connection
	pingCtx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresStore{
		db:     db,
		logger: logger,
	}, nil
}

// Close closes the database connection
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) ApplyBatch(ctx context.Context, ops []metadata.Op) error {
	if 0 == len(ops) {
		return nil
	}
	if err := metadata.ValidateOps(ops); nil != err {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if nil != err {
		return fmt.Errorf("failed to begin batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, op := range ops {
		if err := applyBatchOp(ctx, tx, op); nil != err {
			return err
		}
	}

	if err := tx.Commit(); nil != err {
		return fmt.Errorf("failed to commit batch: %w", err)
	}
	return nil
}

func applyBatchOp(ctx context.Context, tx dbQuerier, op metadata.Op) error {
	switch op.Kind {
	case metadata.OpCreate:
		return createInode(ctx, tx, op.Metadata)
	case metadata.OpUpdate:
		return updateInode(ctx, tx, op.Metadata)
	case metadata.OpUpdateIfMatch:
		return updateInodeIfMatch(ctx, tx, op.Metadata, op.ExpectedETag)
	case metadata.OpDelete:
		return deleteInode(ctx, tx, op.Path)
	case metadata.OpRename:
		return renameInode(ctx, tx, op.Path, op.NewPath)
	case metadata.OpCreateErasureInfo:
		return createErasureInfo(ctx, tx, op.Path, op.ErasureInfo)
	case metadata.OpDeleteErasureInfo:
		return deleteErasureInfo(ctx, tx, op.Path)
	}
	return fmt.Errorf("%w: unknown kind %q", metadata.ErrInvalidOp, op.Kind)
}
