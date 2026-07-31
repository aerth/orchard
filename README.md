# orchard ai

General, extensible, multi-purpose AI Agent orchestration harness system.

This repo contains the Orchard library, and the basic `orchard-io` command (stdin/stdout).

There is also an `orchard-chat` repo usually located at ../orchard-chat/ for the Chat TUI application.

## Basics

The `chat.Ext` structure is used to contain Messages, Tools, and Hooks used in the loop.

While some applications may use it with only a single Handler, hook functions are available at every step in the process to customize or short-circuit the process.

