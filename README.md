# orchard ai

**tiny agent library** (tool orchestration, agentic harness)

Made for local LLM, such as **ollama** or **llama.cpp**.

No telemetry or advertisements.

Uses [mllm](https://github.com/aerth/mostly/blob/master/mllm/) as LLM client,
and GORM (SQL) for optional persistence.

## Tools

Your program should define it's own tools.
The examples use a simple [bash tool](https://github.com/aerth/orchard/blob/master/orchard_tools.go#L55) that is convenient for testing, but is both dangerous and limiting.

Orchard can use dynamic toolboxes. See [orchard-tools](https://github.com/aerth/orchard-tools) for an example toolbox repo.

## **Warnings**

- Runs shell commands, sends system information to the LLM provider you choose.
- Executes code provided by LLM with no confirmations.

## Examples

- [orchard-tui](https://github.com/aerth/orchard-tui): TUI agent interface using bubbletea UI library
- [orchard_test.go](./orchard_test.go): for example library usage.
- [orchard-io](./cmd/orchard-io/): minimal example, read stdin, print to stdout
- [orchard-tools](https://github.com/aerth/orchard-tools): a tool scripts repository

## Work In Progress

Many components have not made it into this source tree yet. These include:

- ~Dynamic toolboxes (bash/python scripts for tool.Exec)~ ✅
- Confirmation (yes/no/cancel)
- Source tree in sticky system message
- conversation compression/truncate

This package, like [mllm](https://github.com/aerth/mostly), will remain lightweight and versatile, and the API is mostly stable.

For Documentation, refer to source code or [pkg.go.dev](https://pkg.go.dev/github.com/aerth/orchard)
[Latest source](https://github.com/aerth/orchard)
