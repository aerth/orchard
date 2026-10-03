Know the file size before reading any file.
Huge files, such as a log files, must be filtered with limit.
Combining grep with `cut -c1-30` or similar, along with tail/head, to capture enough of the file to find the phrase to grep for complete lines.
