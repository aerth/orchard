# [orchard ai](https://github.com/aerth/orchard)

**tiny agent library** (tool orchestration, agentic harness)

Made for local LLM, such as **ollama** or **llama.cpp**.

No telemetry or advertisements.

Uses the lightweight [mllm](https://github.com/aerth/mostly/blob/master/mllm/) package for LLM client,
and GORM (SQL) for optional persistence.

## Library

Create a `Config` and call `Create` (or `CreateConvo`).

By default, streams to `stdout`, logs to default `slog` logger. This can be customized for each application.

Minimal complete example. (`go test -v -run Test04`)

```go
	cfg := orchard.Config{Toolbox: orchard.AllTools}
	_, err := orchard.CreateConvo(ctx, cfg, mllm.ChatMessages{{Role: "user", Content: "explain this repo"}})
```

### Runtime Configuration 

Environmental variables configure LLM endpoint/model at runtime (see mllm package).

Optionally, use mllm.Flags and/or orchard.Flags functions to add more runtime configuration via command-line flags.

## Tools

Tools are easy to implement, in either Go, Bash, or Python.

Orchard can use dynamic toolboxes. See [orchard-tools](https://github.com/aerth/orchard-tools) for an example toolbox repo.

Your program SHOULD replace `AllTools` (default) tools, or rely solely on dynamic toolboxes (e.g. `orchard.AllTools = toolbox`).

The examples use a simple [bash tool](https://github.com/aerth/orchard/blob/master/pkg/builtintools/bash.go) that is convenient for testing.

Dynamic toolboxes are collections of scripts and are scanned at runtime, using the list of paths in `ORCHARD_TOOLS` env (separated by `:` similar to how `$PATH` works).

Dynamic toolboxes may contain `.env` files that will be used for tools in that toolbox.

## Examples

To get started quickly with no TUI, try `make run` after exporting the required LLM_MODEL and LLM_HOST env variables

- [orchard-tui](https://github.com/aerth/orchard-tui): TUI agent interface using bubbletea UI library
- [orchard_test.go](./orchard_test.go): for example library usage.
- [orchard-io](./cmd/orchard-io/): minimal example, read stdin, print to stdout
- [orchard-tools](https://github.com/aerth/orchard-tools): a tool scripts repository

## Hooks

- `PreTextHook`: Runs before generating the message (useful for UI to push a new message to the view).
- `PreToolHook`: Can modify or deny tool arguments before execution.
- `PostToolHook`: Can modify or deny tool outputs.
- `NonTextHook`: A UI-specific hook for tool outputs.
- `FinishHook`: Runs after the entire loop is finished.

## Work In Progress

Many components have not made it into this source tree yet. These include:

- Confirmation (yes/no/cancel)
- Source tree in sticky system message
- conversation compression
- reading SKILLS or other runtime prompts

This package, like [mllm](https://github.com/aerth/mostly), will remain lightweight and versatile.

For Documentation, refer to source code or [pkg.go.dev](https://pkg.go.dev/github.com/aerth/orchard)

[Latest source](https://github.com/aerth/orchard)
