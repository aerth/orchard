//go:build cgo

package main

import (
	"log"
	"log/slog"
	"time"

	"github.com/aerth/mostly/mllm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gdb *gorm.DB

func opendb(dsn string) gorm.CreateInterface[mllm.ChatMessage] {
	slog.Info("opening db", "thread", mllm.DefaultThread)
	gormOptions := &gorm.Config{
		TranslateError: true,
		Logger: logger.NewSlogLogger(slog.Default(), logger.Config{
			SlowThreshold:             time.Millisecond * 700,
			Colorful:                  true,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
	}
	db, err := gorm.Open(sqlite.Open(dsn), gormOptions)
	if err != nil {
		log.Fatalf("%+v", err)
		//	return fmt.Errorf("open sqlite %q: %w", dbfile, err)
	}
	gdb = db
	return gorm.G[mllm.ChatMessage](db)
}

func closedb() {
	CloseDB(gdb)
}
