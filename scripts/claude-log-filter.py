#!/usr/bin/env python3
"""Filter Claude stream-json output into readable text."""
import json
import sys

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        msg = json.loads(line)
    except json.JSONDecodeError:
        print(line)
        continue

    t = msg.get("type")

    if t == "assistant":
        content = msg.get("message", {}).get("content", [])
        for block in content:
            if block.get("type") == "text":
                print(f"  {block['text']}")
            elif block.get("type") == "tool_use":
                name = block.get("name", "")
                inp = block.get("input", {})
                if name == "Bash":
                    print(f"  > {inp.get('command', '')}")
                elif name == "Edit":
                    path = inp.get("file_path", "")
                    print(f"  [edit] {path}")
                elif name == "Write":
                    path = inp.get("file_path", "")
                    print(f"  [write] {path}")
                elif name == "Read":
                    path = inp.get("file_path", "")
                    print(f"  [read] {path}")
                elif name == "Glob":
                    print(f"  [glob] {inp.get('pattern', '')}")
                elif name == "Grep":
                    print(f"  [grep] {inp.get('pattern', '')}")
                elif name == "Agent":
                    print(f"  [agent] {inp.get('description', '')}")
                else:
                    print(f"  [{name}]")

    elif t == "system":
        sub = msg.get("subtype", "")
        if sub == "task_progress":
            desc = msg.get("description", "")
            if desc:
                print(f"  ... {desc}")
