package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"uuid"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/toolboxes"
)

var client orchard.Config

func main() {
	prefix := ""
	mllm.Flags()
	orchard.Flags()
	flag.StringVar(&prefix, "system", prefix, "for example, orchard-io -system 'explain this file' < README.md")
	flag.Parse()
	orchard.Slog = mllm.Slog
	_ = godotenv.Load()
	client.E = mllm.Default()
	client.Toolbox = orchard.AllTools
	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		box, err := toolboxes.NewToolboxFS(dir)
		if err != nil {
			log.Fatalln(err)
		}
		client.Toolbox = box.AllTools()
		//	for _, v := range client.Toolbox {
		//		println("TOOL AVAILABLE:", v.GetName())
		//	}
	}
	ctx := context.Background()
	threadID := uuid.MustParse("7e0330fc-bb4a-48d7-aae5-bd666db0734a")
	client.Writer = os.Stdout

	// one round
	client.ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		if prefix != "" {
			prefix += " "
		}
		return mllm.ChatMessages{{
			Role:    "user",
			Content: prefix + string(input),
		}}, nil
	}
	_, err := orchard.Create(ctx, threadID, client)
	if err != nil {
		log.Fatalln(err)
	}
}
