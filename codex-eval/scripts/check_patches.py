#!/usr/bin/env python3
"""Verify codex-eval patch files: order, LF endings, and SHA-256 manifest."""

import argparse
import hashlib
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PATCH_DIR = ROOT / "patches"
MANIFEST = PATCH_DIR / "SHA256SUMS"
NAME_RE = re.compile(r"^(\d{4})-[a-z0-9-]+\.patch$")
HASH_RE = re.compile(r"^[0-9a-f]{64}$")


def normalized_bytes(path):
    data = path.read_bytes()
    if b"\r\n" in data:
        data = data.replace(b"\r\n", b"\n")
    return data


def load_manifest():
    if not MANIFEST.is_file():
        raise SystemExit(f"Missing manifest: {MANIFEST}")
    entries = []
    for number, line in enumerate(MANIFEST.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        parts = line.split("  ", 1)
        if len(parts) != 2 or not HASH_RE.fullmatch(parts[0]):
            raise SystemExit(f"Invalid SHA256SUMS line {number}: {line!r}")
        entries.append((parts[0], parts[1]))
    return entries


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tree", help="Optional Sub2API worktree; also run git apply --check --binary")
    args = parser.parse_args()

    patches = sorted(PATCH_DIR.glob("*.patch"))
    if len(patches) != 7:
        raise SystemExit(f"Expected 7 patches, found {len(patches)}")
    for index, patch in enumerate(patches, 1):
        match = NAME_RE.fullmatch(patch.name)
        if not match or int(match.group(1)) != index:
            raise SystemExit(f"Patch order is not 0001..0007: {patch.name}")

    entries = load_manifest()
    if [name for _, name in entries] != [patch.name for patch in patches]:
        raise SystemExit("SHA256SUMS entries do not match patch order")

    import subprocess

    for (expected, name), patch in zip(entries, patches):
        data = normalized_bytes(patch)
        if b"\r" in data:
            raise SystemExit(f"Patch contains CR line endings: {patch.name}")
        actual = hashlib.sha256(data).hexdigest()
        if actual != expected:
            raise SystemExit(f"Hash mismatch for {patch.name}: expected {expected}, got {actual}")
        print(f"OK {name} {actual}")
        if args.tree:
            result = subprocess.run(
                ["git", "apply", "--check", "--binary", str(patch)],
                cwd=args.tree,
                text=True,
                capture_output=True,
            )
            if result.returncode != 0:
                raise SystemExit(f"git apply --check failed for {name}: {result.stderr.strip()}")
            print(f"OK {name} applies to {args.tree}")
    print(f"Verified {len(patches)} patches.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, SystemExit) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
