# orchard ai

**tiny agent library** (tool orchestration, agentic harness)

Made for local LLM.

Uses [mllm](https://github.com/aerth/mostly/blob/master/mllm/) as LLM client,
and sqlite (GORM) for optional persistence.

## Tools

Your program should define it's own tools.
The examples use a simple [bash tool](https://github.com/aerth/orchard/blob/master/orchard_tools.go#L55) that is convenient for testing, but is both dangerous and limiting.

## **Warnings**

Watch output and logs carefully!!

- Is unsafe.
- Runs shell commands.
- Is dangerous.
- Executes code provided by LLM.

## Examples

- [orchard-tui](https://github.com/aerth/orchard-tui) TUI using bubbletea UI library
- [orchard_test.go](./orchard_test.go) for example usage.
- [orchard-io](./cmd/orchard-io/) read stdin, print to stdout

## Work In Progress

Many components have not made it into this source tree yet. These include:

- Dynamic toolboxes (bash/python scripts for tool.Exec)
- Confirmation (yes/no/cancel)
- Source tree in sticky system message
- conversation compression/truncate

This package will remain lightweight and versatile, and the API is mostly stable.

For Documentation, refer to source code or [pkg.go.dev](https://pkg.go.dev/github.com/aerth/orchard)
[Latest source](https://github.com/aerth/orchard)
