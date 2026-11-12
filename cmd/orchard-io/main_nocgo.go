//go:build !cgo

package main

import (
	"github.com/aerth/mostly/mllm"
	"gorm.io/gorm"
)

func opendb(string) gorm.CreateInterface[mllm.ChatMessage] {
	return nil
}

func closedb() {}
