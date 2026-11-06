package main

import (
	"context"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
)

var client mllm.E

func main() {
	_ = godotenv.Load()
	client = mllm.Default()
	ctx := context.Background()
	_, _ = client.Generate(ctx, "", "say hello world in esperanto", false)
}
