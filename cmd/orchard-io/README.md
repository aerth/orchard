# orchard-io command

Examples

No spin, No thinking, Use a persistent thread, clear thread (in DB) before starting.
Streams generated message content to standard output, reads new user message from standard input.
Produces a final message after running tools.

```bash
NO_COLOR=1 orchard-io -thread foo -clear -nospin <<<"hi"
```

Similar, but show tool outputs and reasoning.

```bash
NO_COLOR=1 orchard-io -thread foo -clear -nospin --show-toolouts --show-thinking <<<"hi"
```

