# ORCHARD {{.Version}}

Messages have UUID, Timestamp, and MessageOptions.
Parts of these messages are marshalled only partially (json.RawMessage, []json.Marshaler, or map[string]json.RawMessage)
A message may begin a fork, or gather messages from elsewhere.
Tools are, minimally, a bash (or python) script containing a properly formatted metadata line.
The scripts are each parsed into struct OrchTool, which is a json.Marshaler (for llm API) and has Exec method to run the tool (in the orchestration loop). 

Essentially, the orchestration goes: 

1. Accept user message, load the previous conversation history from database by `ThreadID`,
2. Handle commands, 'exit' requests, and other non-generative interactions.
3. send llm request (history + newmsg)
4. if tools in response, run Exec on matched tools and add `toolout` lists and loop back to 3


