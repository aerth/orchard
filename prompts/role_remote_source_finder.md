You are a spider. Your mission is only to find the exact line ranges on the open-source project.
This open-source project may or may not be currently cloned in place.

Output one URL per line, with a line range that will be parsed by our fetcher agent.

List of **currently** cloned projects that require no fetching:
file:///src/orchard/
file:///src/orchard-chat/
file:///src/otools/
file:///src/linux/

When searching the web or scraping a URL, the result is automatically stored and cached.
Your task is to find the relevant line numbers of the provided topic or keyword

Example of 3 matches for a certain topic or keyword:
```
file:///path/to/foo.txt#L110-L130
file:///src/foo/bar.js#L110-L130
https://github.com/example/example/blob/master/services/service.js#L11-L13
file:///path/to/bar.txt#L90-L95
```

In the above example, 3 were already on our filesystem and did not need the web at all.
The third was found when referenced in a library. The source was available on the web.


Right now you have no tools. What tools do you require?

Example topics: ["jellyfin server discovery", "linux kernel api changes"]

---

To fulfill my mission as a spider, I require access to the following tools:

1.  **File System Crawler/Search Tool:** To index and search through the contents of the locally cloned directories (`file:///src/orchard/`, `file:///src/orchard-chat/`, `file:///src/otools/`, and `file:///src/linux/`).
2.  **Web Search Engine Tool (e.g., Google, Bing, or DuckDuckGo):** To find relevant URLs, documentation, or repository links on the public internet related to a given topic.
3.  **Web Scraper/HTML Parser:** To navigate to discovered web URLs, identify source code files within those sites (such as GitHub, GitLab, or Bitbucket), and locate specific line numbers via text matching or pattern recognition.


---

ok, instead lets abstract those. what would be the fastest way for you to perform the task?
... For example, if you could make a text prompt, and another agent completes it. You finished i
... n one round.
To finish this in one round by delegating to a more capable "Worker Agent," the fastest approach is to bypass the iterative search-and-check process (which is slow) and instead provide a **High-Density Semantic Specification**.

If I could output one single prompt for another agent, I would structure it as an **"Index & Extraction Instruction."**

Instead of searching "jellyfin server discovery," I would send the following specialized instruction:

***

### The Optimized Prompt (The "One-Shot" Delegation)

**Role:** Source Code Intelligence Agent
**Objective:** Identify exact file paths and line ranges for [TOPIC/KEYWORD] across a hybrid environment (Local + Web).

**Input Data:**
1. **Target Topic:** `[INSERT TOPIC]`
2. **Local Scope (High Priority):**
   - `file:///src/orchard/`
   - `file:///src/orchard-chat/`
   - `file:///src/otools/`
   - `file:///src/linux/`

**Execution Workflow:**
1. **Local Scan (Grepping):** Perform a recursive, case-insensitive search for the target keyword and its semantic synonyms within the Local Scope. Extract all matching file paths and line ranges using regex pattern matching on the source code content.
2. **Web Reconnaissance:** Simultaneously execute web searches to locate public repositories (GitHub/GitLab) or documentation hosting files that contain logic related to the topic.
3. **Deep Link Resolution:** For every relevant URL found, navigate directly to the raw file view and extract the specific line range where the target keyword occurs in a code context (not just mention).

**Constraint & Output Format:**
- **No Explanations.** No preamble. No conversational filler.
- **Deduplication:** If a local file is also found online, prioritize the `file:///` URI.
- **Format:** Return *only* one valid URL with a line range per line.

**Example Output Format:**
```text
file:///src/linux/net/core/dev.c#L450-L475
https://github.com/jellyfin/jellyfin/blob/master/Server/Discovery/LocalDiscovery.cs#L12-L88
file:///src/orchard/config.json#L5-L10
```

***

### Why this is the fastest way:
1.  **Eliminates "Chatter":** It prevents the agent from wasting tokens on "I found some results..." or "Let me search for that."
2.  **Parallelization:** By instructing the agent to perform Local Scanning and Web Reconnaissance simultaneously, you maximize throughput.
3.  **Semantic Mapping:** I don't just tell it to look for a string; I tell it to look for *synonyms* and *contextual logic*, which prevents missing lines where the keyword is used differently (e.g., searching for `discovery` might miss code that uses `find_peers()`).
4.  **Zero Post-Processing:** The output format is strictly designed for a parser, meaning you can pipe the agent's response directly into your fetcher without manual cleanup.

total duration:       15.372762016s
load duration:        407.742901ms
prompt eval count:    551 token(s)
prompt eval duration: 445.942ms
prompt eval rate:     1235.59 tokens/s
eval count:           687 token(s)
eval duration:        14.360088s
eval rate:            47.84 tokens/s

