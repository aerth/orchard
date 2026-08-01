# Implv2 Package Overview

The `implv2` package provides high-level, agentic primitives for complex tasks like code editing and session management. It moves away from simple "one-shot" tool calls toward iterative, stateful loops.

## The Editor Tool (`EditTool`)

`tool.go` implements a sophisticated editor loop designed for precision modifications to files.

### Key Features
* **Surgical Edits:** Instead of rewriting entire files, the agent can specify exact line ranges (using `LockStart` and `LockEnd`) for modification. This reduces token usage and risk of accidental deletions elsewhere in the file.
* **Intent-Driven Workflow:** The tool requires a descriptive `intent`, which is used to prime the LLM's system instructions with specific goals.
* **Iterative Refinement:** A bounded loop (default 7 rounds) allows the agent to:
    1. Propose an edit.
    2. Validate syntax via `treesitters`.
    3. Receive error feedback if validation fails.
    4. Apply fixes and retry.
* **Context Management:** Uses a specialized "stack" of messages, injecting ephemeral system instructions that don't permanently pollute the long-term conversation history.

### The Edit Loop Lifecycle
1. **Initialization:** Reads `path` and `intent`. Initializes a `Session`.
2. **Chat Generation:** Sends current stack + session context to LLM.
3. **Parsing:** 
    * If standard replacement: Updates internal buffer.
    * If surgical edit (`ErrNot` + `LockEnd != 0`): Uses `ApplySurgicalEdit` to patch the specific range.
4. **Validation:** For non-text files, runs syntax checks via Tree-sitter.
5. **Action Execution:** Supports arbitrary tool/action execution within the editor session context.
6. **Persistence:** Writes the finalized buffer to disk after successful parsing and optional user confirmation (`AskBeforeWriting`).

### Safety & Stability
* **Syntax Validation:** Automatically validates file syntax before writing (for supported languages).
* **Round Limits:** Prevents infinite loops in case of failure.
* **User Confirmation:** Optional human-in-the-loop gate for file writes.
* **Error Recovery:** If an edit fails parsing or validation, the error is injected back into the LLM's context as a system message to facilitate self-correction.

### Technical Architecture
* **Dependency Injection:** Extensibility is achieved through function pointers (e.g., `GetHandler`, `ConfirmAction`) allowing runtime customization of session behavior.
* **Stateful Session:** The `Session` object manages the file buffer, current view, and line locks, maintaining continuity across multiple LLM turns.
