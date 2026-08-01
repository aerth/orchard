# Package Analysis: `pkg/builtins/implv2`

The `implv2` package implements a state-machine-driven agent for surgical file editing. It is designed to minimize LLM context usage and error rates by forcing interaction through highly structured protocol commands rather than arbitrary text generation.

## Core Concept: The Protocol
Unlike generic chat interfaces, `implv2` operates on a strict command-response loop. This prevents "hallucinated" edits where an LLM tries to rewrite entire files and instead forces it to interact with the file via discrete operations.

### Command Set (Operator Keywords)
The system uses six explicit operator keywords to maintain control:

| Operator | Argument | Description |
| :--- | :--- | :--- |
| `SELECT_FILE` | `<filename>` | Loads a target file into memory snapshot. |
| `NEXT_CHUNK` | *None* | Moves the viewport forward by one chunk size. |
| `PREV_CHUNK` | *None* | Moves the viewport backward by one chunk size. |
| `GOTO` | `<search_string>` | Jumps the view window to center on a literal string match. |
| `LOCK` | `<start>-<end>` | Sets 1-based line indices for surgical editing (Edit Mode). |
| `CANCEL` | *None* | Reverts from Edit mode back to Read/Navigation mode. |

## State Machine & Lifecycle

The interaction follows a rigid state machine lifecycle managed by the `Session` struct:

### 1. READ Phase (Navigation)
In this phase, the agent explores the file. The LLM is provided with a "paginated viewport" containing only a subset of lines (defined by `ChunkSize`). This prevents token exhaustion and focuses attention on the relevant code block. Each line in the view is prefixed with its absolute 1-based index.

### 2. LOCK Phase (Preparation)
Once the agent identifies the target range, it must issue a `LOCK <start>-<end>` command. The system validates:
- That the range exists within the current file bounds.
- That the indices are valid integers.
- That the agent is not attempting to lock more than `MaxLinesLocked` (currently 12).

Upon success, the session transitions into **EDIT mode**.

### 3. EDIT Phase (Surgical Modification)
Once locked, the system expects a stream of raw code lines. These lines will replace the existing content between the `LockStart` and `LockEnd` indices exactly. The agent cannot "append" or "prepend" in this mode; it performs an atomic replacement of the specified slice.

## Safety & Robustness Mechanisms

*   **Strict Parsing:** The `OutputParser` rejects any response containing leading whitespace, markdown (backticks/asterisks), or conversational filler to ensure predictable execution.
*   **Bounded Edits:** By enforcing a small `MaxLinesLocked`, the system prevents "catastrophic rewrites" where an LLM error could wipe out large portions of a file in one turn.
*   **Context Density Control:** The paginated viewport (`CurrentView`) ensures the LLM only processes relevant slices of code, significantly reducing noise and improving tool-call accuracy.
*   **Error Isolation:** Errors (like invalid line numbers or parsing failures) are returned as structured `Result` objects back to the orchestration loop, allowing for graceful recovery or user intervention.

## Implementation Details

- **Complexity:** Command execution (specifically `GOTO`) is $O(N)$ relative to file length.
- **Communication:** The system uses a specialized `ActionRunner` interface to decouple the protocol logic from the actual LLM and filesystem implementations.
