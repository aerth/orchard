# orchard-io

agentic command line shell

## **Warnings**

The example program is very basic.

- Runs shell commands, sends system information to the LLM provider you choose.
- Executes code provided by LLM with no confirmations.
- Uses ORCHARD_TOOLS, ORCHARD_DB, LLM_MODEL, LLM_HOST, LLM_API_KEY environmental variables
- Reads ~/.config/orchard/env file
- Always has bash tool. Use `--no-bash` to restrict only to the toolbox tools.

## example usage

Each of these different ways should essentially *do* the same thing. The agent should read the file and (only) a summary printed to stdout.

```bash
# redirect stderr logs to jq (or your preferred JSON log prettifier)
# stream output to stdout
bin/orchard-io --system "You are a file summarizer. Plaintext, No Markdown."<<<"./README.md" 2> >(jq -c)

# hide stderr logs
# stream output to stdout
bin/orchard-io <<<"summarize ./README.md in plain text no markdown" 2>/dev/null

# capture and use output, print toolcalls to stderr
summary=$(bin/orchard-io <<<"summarize ./README.md in one sentence")
echo $summary
```

