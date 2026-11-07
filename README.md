# orchard ai

tiny agent library (tool orchestration, agentic harness)

## Current Status

Currently, the only exposed tool is bash. This gives the agent extraordinary abilities.

Optional persistence via sqlite database file.

## **Warnings**

Watch output and logs carefully!!

- Is unsafe.
- Runs shell commands.
- Is dangerous.
- Executes code provided by LLM.


## Examples

- [orchard-tui](https://github.com/aerth/orchard-tui) TUI using bubbletea UI library
- [orchard_test.go](./orchard_test.go) for example usage.

## Work In Progress

Many components have not made it into this source tree yet. These include:

- Dynamic toolboxes (bash/python scripts for tool.Exec)
- Confirmation (yes/no/cancel)
- tree in sticky system message
- conversation compression/truncate

This package will remain lightweight and versatile, and the API is mostly stable.

For Documentation, refer to source code or [pkg.go.dev](https://pkg.go.dev/github.com/aerth/orchard)

