// Package postgres for running migrate
package postgres

import (
	"fmt"
	"log/slog"
	"os"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// PostgresDBStore implements Store using PostgreSQL.
type PostgresDBStore struct {
	db *gorm.DB
}

func (s *PostgresDBStore) DB() *gorm.DB {
	if s.db == nil {
		panic("no underlying db")
	}
	return s.db
}

func UnscopedDB[T any](db *gorm.DB) gorm.ChainInterface[T] {
	return gorm.G[T](db).Scopes(func(tx *gorm.Statement) { tx.Unscoped = true })
}

// NewPostgres creates a Store backed by PostgreSQL.
func NewPostgres(additionalMigrateTypes ...any) (*PostgresDBStore, error) {
	diale := postgres.Open(os.Getenv("POSTGRES_DSN"))
	gdb, err := gorm.Open(diale)
	if err != nil {
		slog.Error("opening postgres db", "err", err)
		return nil, err
	}
	db, err := gdb.DB() //sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	if len(additionalMigrateTypes) == 0 {
		return &PostgresDBStore{db: gdb}, nil
	}
	slog.Debug("automigrations...", "types", typenames(additionalMigrateTypes...))
	err = gdb.Migrator().AutoMigrate(additionalMigrateTypes...)
	if err != nil {
		return nil, err
	}
	return &PostgresDBStore{db: gdb}, nil
}

func typenames(v ...any) []string {
	a := make([]string, len(v))
	for i := range v {
		a[i] = fmt.Sprintf("%T", v[i])
	}
	return a
}
