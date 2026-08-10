# orchard ai

General, extensible, multi-purpose AI Agent orchestration harness system.

This repo contains the Orchard library, and the basic `orchard-io` command (stdin/stdout).

There is also a separate `orchard-chat` repo for TUI,  usually located at ../orchard-chat/

## Basics

The `chat.Ext` structure is used by callers to customize *the loop*.

While some applications may use use a single Handler, hook functions are
available at every step in the process to customize or short-circuit the process.

## Runtime Configuration


| Variable | Description |
| :--- | :--- |
| `ORCHARD_TREE` | Path to the orchestration tree directory (may be empty to use embedded fs). |
| `ORCHARD_TOOLS` | Path to the tools library. (may be empty to use embedded fs)|
| `OLLAMA_MODEL` | Default model name for LLM requests. |
| `OLLAMA_THINK` | Default reasoning level for LLM requests. |
| `POSTGRES_DSN` | Connection string for PostgreSQL database, used to store conversations. |

## Command-line flags

See [./pkg/global/flags.go]
