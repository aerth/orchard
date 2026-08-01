# Orchard Tooling Standards & Test Data Analysis

## Overview
This document tracks the technical specifications and integration requirements for tools within the Orchard ecosystem, specifically focusing on patterns observed in `testdata/`.

## Tool Integration Requirements

### 1. Argument Handling (Positional vs. Environment)
Orchard's tool execution (`pkg/loaders/toolboxes/toolbox_exec.go`) supports both positional arguments and environment-based input injection for large payloads.

*   **Positional Arguments:** Small, discrete parameters passed directly to the executable.
*   **Large Payload Injection:** For high-entropy or long text inputs (e.g., embeddings), tools should be designed to read from an environment variable (typically ``) to avoid shell command length limitations ().

### 2. Analysis of Test Tools (`testdata/`)

#### `find_occurrences.py`
- **Type:** Python Wrapper for .
- **Functionality:** Performs regex searches with context expansion logic. If matches are below a threshold (< 5), it appends 20 lines of context to facilitate LLM reasoning.
- **Required Tool Definition Schema:**
  - Parameter 1: `pattern` (regex string)
  - Parameter 2: `path` (search directory/file)
- **Optimization Note:** Future iterations should implement explicit file type filtering (`-t`) to reduce token consumption in the orchestration loop.

#### `generate_embeddings.bash`
- **Type:** Bash Wrapper for Ollama
- **Functionality:** Generates 256-dimension JSON embeddings using `nomic-embed-text:v1.5`.
- **Input Strategy:** Utilizes the pattern `INPUT="${1:-$INPUT_FILE}"` to support both direct positional arguments and environment-based injection for large text blocks.
- **Required Tool Definition Schema:**
  - Parameter 1: `input` (string)

## Implementation Checklist for New Tools
- [ ] Metadata header present (per `ORCHARD.md`).
- [ ] Exit codes used semantically (distinguish between "No Results" and "Execution Error").
- [ ] Support for large inputs via environment variable injection is prioritized for text-heavy tasks.
