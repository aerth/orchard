#!/bin/bash

INPUT="${1:-$INPUT_FILE}"

if [ -z "$INPUT" ]; then
    echo "Error: No input provided via argument or $INPUT_FILE" >&2
    exit 1
fi

echo "$INPUT" | ollama run --verbose --format json --dimensions 256 'nomic-embed-text:v1.5' nomic-embed-text:v1.5