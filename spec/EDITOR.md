# editor

the agent begins the file-editing session (using a tool call such as edit_file with the filename as argument)

the idea is the keep the big file content in the conversation for as few amount of rounds as possible.
when editing session is finished, only the diff is presented in the post-editing conversation context.

1. the conversation ([]message) is condensed (if needed)
2. the file is copied for editing
3. system message is appended to the convo, explaining that we are now in a file-viewing session (contrasted with file-editing).

View mode:
keywords available: 
  - GOTO [linenumber]
  - LOCK [[linenumber]-][line-number]]  (single line or range. begins editing)
  - EXIT (shows diff for confirmation, then writes)

Edit mode:
keywords available: 
  - SAVE 
  - CANCEL
Any text output that is not a reserved keyword, replaces the locked lines.


For example, 
Given file:
1. Hello
2. , World
3. !!!
LOCK 1-3
Hello, World!!!
SAVE


---

