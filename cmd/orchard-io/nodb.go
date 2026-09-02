//go:build !postgres

package main

import (
	"m/orchard/pkg/dbstore"
	"m/orchard/pkg/types/chatmessages"
)

func NewPostgres(chatMessage *chatmessages.ChatMessage) (*dbstore.Store, error) {
	return nil, nil
}
