// Package persistence stores focused local application and evaluation records in SQLite through GORM.
package persistence

// This file opens, migrates, configures, and closes the concrete application database.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const SchemaVersion = 3

// Store owns the application's concrete SQLite connection and focused persistence operations.
type Store struct {
	db *gorm.DB
}

// Open creates the database directory, opens SQLite, applies connection settings, and migrates known tables.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	database, err := gorm.Open(sqlite.Open(absolute), &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("access SQLite connection: %w", err)
	}
	sqlDatabase.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if result := database.Exec(statement); result.Error != nil {
			_ = sqlDatabase.Close()
			return nil, fmt.Errorf("configure SQLite database: %w", result.Error)
		}
	}
	if err := database.AutoMigrate(
		&PortfolioSnapshotRecord{},
		&PortfolioNAVHistoryRecord{},
		&PortfolioNAVImportRecord{},
		&PromptExecutionRecord{},
		&EvaluationOutputRecord{},
		&OrderAuditRecord{},
	); err != nil {
		_ = sqlDatabase.Close()
		return nil, fmt.Errorf("migrate SQLite database: %w", err)
	}
	return &Store{db: database}, nil
}

// Close releases the underlying SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	database, err := s.db.DB()
	if err != nil {
		return err
	}
	return database.Close()
}
