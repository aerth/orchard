#!/usr/bin/env python3
import subprocess
import sys
import argparse

def main():
    parser = argparse.ArgumentParser(description='Find occurrences and provide context.')
    parser.add_argument('pattern', help='Regex pattern to search for')
    parser.add_argument('path', help='Path to search in (directory or file)')
    args = parser.parse_args()

    try:
        # Get basic matches first
        matches_raw = subprocess.check_output(['rg', '--line-number', args.pattern, args.path], text=True).strip().split('\n')
    except subprocess.CalledProcessError as e:
        print("No matches found.")
        sys.exit(0)

    match_count = len(matches_raw)
    for m in matches_raw:
        print(m)

    if match_count < 5:
        print("\n--- Extended Context (low hit count detected) ---")
        # Provide more context if the pattern is rare
        try:
            context = subprocess.check_output(['rg', '--context=20', args.pattern, args.path], text=True)
            print(context)
        except subprocess.CalledProcessError:
            pass

if __name__ == "__main__":
    main()
