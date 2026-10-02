#!/usr/bin/env python3
"""Check relative Markdown file links; anchors and remote URLs are not checked."""
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

root = Path(__file__).resolve().parents[1]
errors = []
count = 0
for path in root.rglob("*.md"):
    if any(part in {".git", ".cache", ".local", "vendor"}
           for part in path.relative_to(root).parts):
        continue
    content = re.sub(r"```.*?```", "", path.read_text(), flags=re.S)
    for match in re.finditer(r"\[[^\]]*\]\(([^)]+)\)", content):
        target = match.group(1).strip().split()[0].strip("<>")
        url = urlsplit(target)
        if url.scheme or url.netloc or not url.path:
            continue
        count += 1
        if not (path.parent / unquote(url.path)).exists():
            errors.append(f"{path.relative_to(root)}: missing {target}")
if errors:
    print("\n".join(errors))
    sys.exit(1)
print(f"Documentation links OK ({count} relative file links; anchors/remote URLs excluded).")
