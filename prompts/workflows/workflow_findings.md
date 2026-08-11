Use footnotes [1] to designate all references.

When you use footnotes with relative paths, you are essentially creating a **self-contained document**. This is the standard practice for technical documentation (like LaTeX or Markdown files) intended to be stored in a Git repository.

Here is how to structure it professionally using Markdown syntax.

### The "Professional Technical Doc" Example

In this example, imagine you are writing a `README.md` file inside a project folder. Instead of linking to the internet, you are linking to other files within your own documentation folder.

***

# System Architecture Overview

The core engine relies on a distributed consensus algorithm to maintain state across nodes.[^1] This architecture was chosen to ensure high availability even during localized network partitions.[^2]

For a complete breakdown of the mathematical proofs used in our implementation, please refer to our internal whitepaper file located in the documentation folder.

---

### References
[^1]: `./docs/algorithms/consensus_model.md` — Detailed explanation of the Raft implementation.
[^2]: `../specs/network_requirements.pdf` — Specifications for node-to-node communication latency.

***

### Why this is "Best Practice":

1.  **The Notation (`[^1]`):** Using the caret symbol with numbers in brackets is the standard Markdown way to create a footnote. Most modern editors (VS Code, Obsidian, GitHub) will automatically turn these into **clickable hyperlinks**.
2.  **Relative Pathing:**
    *   `./docs/...` tells the computer: "Look in the current folder, then go into the docs folder." This works for everyone who downloads your project.
    *   `../specs/...` tells the computer: "Go up one folder level, then look in the specs folder."
3.  **The "Double-Entry" Method:** Notice that in the Reference section, I provided both the **path** and a **brief description**.
    *   *The path* makes it functional (clickable).
    *   *The description* makes it readable (the user knows what they are opening before they click).
4.  **Clean Body Text:** The main body of your document remains clean and academic. The reader's "flow" isn't interrupted by long file paths like `C:/Users/Project/Docs/Algorithms/...`.

### Pro-Tip for Developers (VS Code / Obsidian)
If you use an editor like **VS Code** or **Obsidian**, if you write the footnote as `./docs/file.md`, it becomes a "live link." You can `Cmd/Ctrl + Click` that reference in your bibliography to instantly jump from your text directly to the source file.
