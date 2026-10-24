package schema

import (
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

//go:embed *.sql
var migrationFS embed.FS

// RunMigrations applies database migrations programmatically on startup
func RunMigrations(dsn string) (err error) {
	// Open database connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create embedded migration source
	source, err := iofs.New(migrationFS, ".")
	if err != nil {
		return fmt.Errorf("failed to open migration source: %w", err)
	}

	// Create database driver
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}

	// Create migrator
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if nil == err && nil != srcErr {
			err = fmt.Errorf("failed to close migration source: %w", srcErr)
		}
		if nil == err && nil != dbErr {
			err = fmt.Errorf("failed to close migration driver: %w", dbErr)
		}
	}()

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

type Migration struct {
	Version int64  `json:"version"`
	Name    string `json:"name"`
}

func AvailableMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		base, isUp := strings.CutSuffix(entry.Name(), ".up.sql")
		if !isUp {
			continue
		}
		prefix, name, hasName := strings.Cut(base, "_")
		if !hasName {
			return nil, fmt.Errorf("migration %s has no version prefix", entry.Name())
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migration %s has an invalid version: %w", entry.Name(), err)
		}
		migrations = append(migrations, Migration{Version: version, Name: name})
	}
	slices.SortFunc(migrations, func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})
	return migrations, nil
}

func CurrentVersion(ctx context.Context, dsn string) (int64, bool, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return 0, false, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	var version int64
	var dirty bool
	err = db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to read migration version: %w", err)
	}
	return version, dirty, nil
}
