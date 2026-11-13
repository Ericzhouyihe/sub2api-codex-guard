#!/usr/bin/env python3
"""Report the compatibility diff between the fork and upstream codex-candy-eval.

Compatibility changes are files that exist in upstream and were modified or
removed. New fork-only files (build/test tooling, docs) are reported separately
and do not count against the "don't rewrite upstream" budget in spec SC-004.
"""

import argparse
import os
import sys
from pathlib import Path

IGNORED_DIRS = {".git", "dist", "build", "node_modules", "__pycache__", ".venv", ".mypy_cache"}


def files(root):
    result = {}
    for current, dirs, names in os.walk(root):
        dirs[:] = [name for name in dirs if name not in IGNORED_DIRS]
        current_path = Path(current)
        for name in names:
            path = current_path / name
            result[path.relative_to(root).as_posix()] = path
    return result


def same_content(left, right):
    """Compare text/bytes while ignoring CRLF vs LF noise in patch files."""
    left_bytes = left.read_bytes()
    right_bytes = right.read_bytes()
    if left_bytes == right_bytes:
        return True
    # Only normalize line endings for text-like files; binary data must match exactly.
    if b"\x00" not in left_bytes and b"\x00" not in right_bytes:
        return left_bytes.replace(b"\r\n", b"\n") == right_bytes.replace(b"\r\n", b"\n")
    return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--upstream", required=True, help="Path to upstream codex-candy-eval checkout")
    parser.add_argument("--fork", default=str(Path(__file__).resolve().parents[1]))
    parser.add_argument("--limit", type=float, default=0.30, help="Maximum compatibility-file ratio (default 0.30)")
    args = parser.parse_args()

    upstream_root = Path(args.upstream).resolve()
    fork_root = Path(args.fork).resolve()
    if not upstream_root.is_dir():
        raise SystemExit(f"upstream not found: {upstream_root}")

    upstream = files(upstream_root)
    fork = files(fork_root)
    changed, added, removed = [], [], []
    for name, path in sorted(fork.items()):
        other = upstream.get(name)
        if other is None:
            added.append(name)
        elif not same_content(path, other):
            changed.append(name)
    for name in sorted(upstream):
        if name not in fork:
            removed.append(name)

    denominator = max(1, len(upstream))
    compatibility_files = len(changed) + len(removed)
    ratio = compatibility_files / denominator
    print(f"upstream files: {len(upstream)}")
    print(f"fork files:     {len(fork)}")
    print(f"modified: {len(changed)}, removed: {len(removed)}, fork-only additions: {len(added)}")
    print(f"compatibility-file ratio: {ratio:.2%} (limit {args.limit:.0%})")
    if changed:
        print("modified upstream files:")
        for name in changed:
            print(f"  {name}")
    if removed:
        print("removed upstream files:")
        for name in removed:
            print(f"  {name}")
    if added:
        print("fork-only files (not counted as compatibility changes):")
        for name in added:
            print(f"  {name}")
    if ratio > args.limit:
        print(f"FAIL: compatibility diff {ratio:.2%} exceeds {args.limit:.0%}", file=sys.stderr)
        return 1
    print("OK: compatibility diff within budget")
    return 0


if __name__ == "__main__":
    sys.exit(main())
