package main

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/aerth/mostly/godotenv"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/toolboxes"
)

func main() {
	ctx := context.TODO()

	// get prompt somehow
	i := slices.Index(os.Args, "--prompt")
	prompt := strings.Join(os.Args[i+1:], " ")
	if i < 0 || strings.TrimSpace(prompt) == "" {
		println("usage: " + os.Args[0] + " --prompt tell me what i can build with this library")
		os.Exit(1)
	}

	// use ~/.config/orchard/env
	_ = godotenv.Load(os.ExpandEnv("${XDG_CONFIG_HOME}/orchard/env"))

	// has bash tool by default, but might have ORCHARD_TOOLS dir
	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		box := loadDynamicTools(dir)
		orchard.AllTools = box // in this example, replace. no bash tool if tools found
		// or:
		//orchard.AllTools = append(orchard.AllTools, box...) // append for keeping bash tool
	}

	// run with defaults
	err := orchard.New().Run(ctx, mllm.ChatMessages{{Role: "user", Content: prompt}})
	if err != nil {
		println("Error:", err.Error())
		os.Exit(2)
	}
}

func loadDynamicTools(dir string) mllm.Tools {
	box, err := toolboxes.NewToolboxFS(dir)
	if err != nil {
		slog.Error("toolbox load", "err", err)
		os.Exit(1)
	}
	return box.AllTools()
}
