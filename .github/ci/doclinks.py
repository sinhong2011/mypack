#!/usr/bin/env python3
"""Check relative links in changed Markdown files. Does not fetch URLs."""

from __future__ import annotations

import re
import sys
from pathlib import Path

LINK = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
SRC = re.compile(r"""(?:src|href)\s*=\s*["']([^"']+)["']""", re.I)
SKIP_PREFIX = ("http://", "https://", "mailto:", "data:", "//")


def changed_markdown(diff_text: str) -> list[str]:
    paths: list[str] = []
    for line in diff_text.splitlines():
        if not line.strip() or line.startswith("#"):
            continue
        parts = line.split("\t")
        status = parts[0]
        candidates = parts[1:]
        if status.startswith(("R", "C")) and len(candidates) >= 2:
            chosen = [candidates[1]]
        else:
            chosen = candidates[:1]
        for raw in chosen:
            path = raw.strip().strip('"')
            if path.endswith(".md"):
                paths.append(path)
    return paths


def link_target(raw: str) -> str | None:
    target = raw.strip().split()[0]
    if not target or target.startswith("#") or target.startswith(SKIP_PREFIX):
        return None
    target = target.split("#", 1)[0].split("?", 1)[0]
    if not target:
        return None
    return target


def missing_links(root: Path, markdown_files: list[str]) -> list[str]:
    problems: list[str] = []
    for rel in markdown_files:
        src = root / rel
        if not src.is_file():
            # Deleted markdown has nothing left to check.
            continue
        text = src.read_text(encoding="utf-8", errors="replace")
        found = LINK.findall(text) + SRC.findall(text)
        for raw in found:
            target = link_target(raw)
            if target is None:
                continue
            if target.startswith("/"):
                dest = root / target.lstrip("/")
            else:
                dest = (src.parent / target).resolve()
            try:
                dest.relative_to(root.resolve())
            except ValueError:
                problems.append(f"{rel}: link escapes the repository: {raw}")
                continue
            if not dest.exists():
                problems.append(f"{rel}: missing {target}")
    return problems


def main(argv: list[str]) -> int:
    if len(argv) != 3 or argv[1] != "--diff":
        print("usage: doclinks.py --diff <name-status-file>", file=sys.stderr)
        return 2
    root = Path(__file__).resolve().parents[2]
    diff = Path(argv[2]).read_text(encoding="utf-8")
    problems = missing_links(root, changed_markdown(diff))
    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
