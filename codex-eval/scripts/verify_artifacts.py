#!/usr/bin/env python3
"""Verify codex-eval build artifacts against contracts/artifact-layout.md."""

import argparse
import hashlib
import re
import struct
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_ROOT = ROOT / "dist" / "build"
TAG_RE = re.compile(r"^v\d+\.\d+\.\d+$")
PLATFORM_RE = re.compile(r"^(linux)-(amd64|arm64)$")
MACHINES = {"amd64": 62, "arm64": 183}


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def check_elf(path, arch):
    data = path.read_bytes()[:20]
    if len(data) < 20 or data[:4] != b"\x7fELF":
        raise ValueError(f"{path.name} is not an ELF file")
    if data[4] != 2 or data[5] != 1:
        raise ValueError(f"{path.name} is not a 64-bit little-endian ELF")
    e_type = struct.unpack_from("<H", data, 16)[0]
    e_machine = struct.unpack_from("<H", data, 18)[0]
    if e_type not in (2, 3):
        raise ValueError(f"{path.name} is not an executable ELF")
    if e_machine != MACHINES[arch]:
        raise ValueError(f"{path.name} machine {e_machine} != {arch}")


def load_sums(directory):
    path = directory / "SHA256SUMS"
    if not path.is_file():
        raise ValueError(f"{directory.name}: missing SHA256SUMS")
    entries = {}
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        parts = line.split("  ", 1)
        if len(parts) != 2 or not re.fullmatch(r"[0-9a-f]{64}", parts[0]):
            raise ValueError(f"{path.name}:{number}: invalid line")
        entries[parts[1]] = parts[0]
    return entries


def verify_platform(directory, tag):
    match = PLATFORM_RE.fullmatch(directory.name)
    if not match:
        raise ValueError(f"Unsupported platform directory: {directory.name}")
    system, arch = match.groups()
    server = f"sub2api-plugins-{tag}-{system}-{arch}"
    installer = f"sub2api-plugins-installer-{tag}-{system}-{arch}"
    sums = load_sums(directory)
    for name, is_installer in ((server, False), (installer, True)):
        path = directory / name
        if not path.is_file() or path.stat().st_size <= 0:
            raise ValueError(f"{directory.name}: missing or empty {name}")
        actual = sha256(path)
        if sums.get(name) != actual:
            raise ValueError(f"{directory.name}: SHA256 mismatch for {name}")
        check_elf(path, arch)
        print(f"OK {directory.name}/{name} {actual} (installer={is_installer})")
    extra = set(sums) - {server, installer}
    if extra:
        raise ValueError(f"{directory.name}: SHA256SUMS lists unknown files: {sorted(extra)}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=str(DEFAULT_ROOT), help="dist/build directory")
    parser.add_argument("--tag", default="v0.2.21", help="upstream tag in artifact filenames")
    parser.add_argument("--platform", action="append", help="platform directory, e.g. linux-amd64")
    args = parser.parse_args()
    if not TAG_RE.fullmatch(args.tag):
        raise SystemExit(f"Invalid tag: {args.tag}")
    root = Path(args.root)
    if not root.is_dir():
        raise SystemExit(f"Artifact root not found: {root}")
    directories = [root / name for name in args.platform] if args.platform else sorted(
        path for path in root.iterdir() if path.is_dir()
    )
    if not directories:
        raise SystemExit(f"No platform directories under {root}")
    for directory in directories:
        verify_platform(directory, args.tag)
    print(f"Verified {len(directories)} platform(s).")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
