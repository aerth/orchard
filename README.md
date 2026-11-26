# [orchard ai](https://github.com/aerth/orchard)

## **tiny ai agent library** (tool orchestration, agentic harness)

Made for local LLM, such as **ollama** or **llama.cpp**, but should work for any `/v1/chat/completions` endpoint.

No telemetry or advertisements. Bring your own tools.

Uses the lightweight [mllm](https://github.com/aerth/mostly/blob/master/mllm/) package for LLM client, and GORM (SQL) for optional persistence.

## Examples

Try `make run` after exporting the required LLM_MODEL and LLM_HOST env variables.

```bash
# example for ollama at localhost
export LLM_HOST=http://127.0.0.1:11434
export LLM_MODEL=gemma4:12b
make run
```

- [orchard-tui](https://github.com/aerth/orchard-tui): TUI agent interface using bubbletea UI library
- [orchard-io](./cmd/orchard-io/): simple example, read stdin, print to stdout (persistence,dynamic tools)
- [orchard-lite](./cmd/orchard-lite/): minimal example, read command line, print to stdout (dynamic tools, no persistence)
- [orchard_test.go](./orchard_test.go): for example library usage.
- [orchard-tools](https://github.com/aerth/orchard-tools): a tool scripts repository

## Library

Orchard is a very small **library** that provides `New() Config`,  has `err := cfg.Run(ctx, convo)` method, which begins loop, calling tools until finished.

Typically, after the loop finishes, a new user message might be added to the `convo`, and then `Run` is called again.

Using Hook functions in `Config`, the application can customize the loop, providing real-time streaming to UI or confirmation of tool calls.

For further customization, the `Generator` and `ToolRunner` can be customized by mo

Minimal Example (bash tool only), LLM_HOST and LLM_MODEL are required environmental variables:

```go
	// unconfigured it streams to stdout, logs json to stderr
	err := orchard.New().Run(ctx, mllm.ChatMessages{{Role: "user", Content: prompt}})
```

By default, streams to `stdout`, logs to default `slog` logger. This can be customized for each application.

Adding native tools is easy:

```go
	cfg := orchard.New()
	cfg.Toolbox = []mllm.Tools{foo, bar}
	err := cfg.Run(ctx, mllm.ChatMessages{
	  {Role: "user", Content: "explain this repo"},
	})
```

## Runtime Configuration

Environmental variables configure LLM endpoint/model at runtime (see [mllm](https://github.com/aerth/mostly) package).

Use `mllm.Flags()` and `orchard.Flags()` to add more runtime configuration via command-line flags.

## Tools

Tools are easy to implement, in either Go, Bash, or Python.

Orchard can use dynamic toolboxes. See [orchard-tools](https://github.com/aerth/orchard-tools) for an example toolbox repo.

Your program SHOULD replace `AllTools` (default) tools, or rely solely on dynamic toolboxes (e.g. `orchard.AllTools = toolbox`).

The examples use a simple [bash tool](https://github.com/aerth/orchard/blob/master/pkg/builtintools/bash.go) that is convenient for testing.

Dynamic toolboxes are collections of scripts and are scanned at runtime, using the list of paths in `ORCHARD_TOOLS` env (separated by `:` similar to how `$PATH` works).

For tools that use API keys or other secrets, the dynamic toolbox directory may contain `.env` files that will be used for tools in that particular toolbox.

## ORCHARD_TOOLS directory structure

```bash
# example file tree
${ORCHARD_TOOLS}/toolboxes/files/grep.bash
${ORCHARD_TOOLS}/toolboxes/web/web_search.bash
```

For now, env files in a toolbox (`web/.env`) is only used for scripts in that toolbox (`web/`),
If no .env file exists in the inner toolbox, the main toolbox .env is used (`${ORCHARD_TOOLS}/.env`)

```bash
# optional env files
# used for 'web' toolbox
${ORCHARD_TOOLS}/toolboxes/web/.env
# used for any toolbox without their own .env file
${ORCHARD_TOOLS}/.env
```

The tool spec is created from the comments at the top of the script (`# Description:`, `# Param:`, `# Feature:`)

For now, in the example programs, the toolbox subdirectories are for organization only and all toolboxes are combined into one for LLM.

```bash
#!/bin/bash
# Description: Use grep (ripgrep) to find line-numbers and filenames for keywords..
# Param: phrase - The text to search for [string] [required]
# Param: path - Path to the file (if omitted, all subdirectories) [string] [optional]
# Param: case_insensitive - Add -i flag if we arent sure about spelling. Causes more false positives! [boolean] [optional]
flags=(--line-number --word-regexp)
if [[ "$3" == "true" ]]; then
	flags+=("--ignore-case")
fi
rg "${flags[@]}" "${1}" "${2:-.}"
```

If the script spec is formatted correctly, the tool will be available to use.
The tool calling is fully abstracted, isolating the llm from the execution logic and tool implementation.

## Hooks

- `PreTextHook`: Runs before generating the message (useful for UI to push a new empty message to the view for streaming into).
- `PreToolHook`: Can modify or deny tool arguments before execution.
- `PostToolHook`: Can modify or deny tool outputs.
- `NonTextHook`: A UI-specific hook for tool output messages to pop into the view.
- `FinishHook`: Runs after the entire loop is finished (either: hit rounds limit or final message contained no tool calls).
- `PreTextHookEditP1`: Able to modify the entire conversation before generation (compression hook)
- `PreTextHookEditP2`: Able to modify the truncated conversation before generation

## Work In Progress

Many components have not made it into this source tree yet. These include:

- Confirmation (yes/no/cancel)
- Source tree in sticky system message
- conversation compression
- reading SKILLS or other runtime prompts

This package, like [mllm](https://github.com/aerth/mostly), will remain lightweight and versatile.

## More Info

For Documentation, refer to source code or [pkg.go.dev](https://pkg.go.dev/github.com/aerth/orchard)
[Latest source](https://github.com/aerth/orchard)
