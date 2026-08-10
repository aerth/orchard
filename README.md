# orchard ai

General, extensible, multi-purpose AI Agent orchestration harness system.

This repo contains the Orchard library, and the basic `orchard-io` command (stdin/stdout).

There is also an `orchard-chat` repo usually located at ../orchard-chat/ for the Chat TUI application.

## Basics

The `chat.Ext` structure is used to contain Messages, Tools, and Hooks used in the loop.

While some applications may use it with only a single Handler, hook functions are available at every step in the process to customize or short-circuit the process.



## Configuration

| Variable | Description |
| :--- | :--- |
| `ORCHARD_TREE` | Path to the orchestration tree directory (may be empty to use embedded fs). |
| `ORCHARD_TOOLS` | Path to the tools library. (may be empty to use embedded fs)|
| `OLLAMA_MODEL` | Default model name for LLM requests. |
| `OLLAMA_THINK` | Default reasoning level for LLM requests. |
| `POSTGRES_DSN` | Connection string for PostgreSQL database, used to store conversations. |
