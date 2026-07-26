# orchard ai

General, extensible, multi-purpose AI Agent orchestration harness. For tasks.


## examples

**The following prompts are extremely simplified for brevity.** 

### example: Chat TUI app

Familiar paned interface for chatting with a coding agent. (tools: all)

### example: Cron job

Every N minutes, check my inboxes for any important urgent messages. For each urgent message, send me an SMS message to remind me to check my email. (tools: check_inbox, send_sms)

### example: Keybinding for STT->LLM->TTS

You are a linux expert who helps the owner manage their computer. (tools: bash_with_confirmation)

### example: Git hook

For each diff, analyze the work and determine if there are any issues that should be immediately looked at. (tools: read_file)

### example: HTTP API endpoint

A user has opened a new Support Ticket. Is this spam? (format: `{"spamminess":{"type":"number","minimum":0,"maximum":10}}`)

### example: Spam mitigation

Use the check_latest tool to read the latest. For each result, delete_message on any spam.

See Also: [./ORCHARD.md]
