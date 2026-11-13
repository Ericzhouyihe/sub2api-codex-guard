#!/usr/bin/env python3
"""Validate and update codex-eval/compatibility.json."""

import argparse
import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECORD = ROOT / "compatibility.json"
PATCHES = ROOT / "patches"
STATUSES = {"planned", "verified", "failed", "deprecated"}
HASH_RE = re.compile(r"^[0-9a-f]{64}$")
VERSION_RE = re.compile(r"^v?\d+\.\d+\.\d+")
DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T")


def patchset_revision():
    digest = hashlib.sha256()
    patches = sorted(PATCHES.glob("*.patch"))
    if len(patches) != 7:
        raise ValueError(f"Expected 7 patches, found {len(patches)}")
    for patch in patches:
        digest.update(patch.read_bytes().replace(b"\r\n", b"\n"))
    return digest.hexdigest()


def validate(record):
    errors = []
    for field in ("id", "upstream_version", "fork_version", "patchset_revision", "status"):
        if not record.get(field):
            errors.append(f"missing required field: {field}")
    if record.get("upstream_version") and not VERSION_RE.fullmatch(str(record["upstream_version"])):
        errors.append("upstream_version must match vX.Y.Z")
    if record.get("patchset_revision") and not HASH_RE.fullmatch(str(record["patchset_revision"])):
        errors.append("patchset_revision must be 64 lowercase hex characters")
    if record.get("status") not in STATUSES:
        errors.append(f"status must be one of {sorted(STATUSES)}")
    if record.get("status") == "verified":
        if not record.get("verified_at"):
            errors.append("verified status requires verified_at")
        if not record.get("artifacts"):
            errors.append("verified status requires at least one artifact")
    for index, artifact in enumerate(record.get("artifacts") or []):
        for field in ("kind", "platform", "filename", "size_bytes", "sha256"):
            if field not in artifact:
                errors.append(f"artifact[{index}] missing {field}")
        if artifact.get("sha256") and not HASH_RE.fullmatch(str(artifact["sha256"])):
            errors.append(f"artifact[{index}].sha256 is invalid")
        if artifact.get("kind") not in ("installer", "server"):
            errors.append(f"artifact[{index}].kind must be installer or server")
    if record.get("verified_at") and not DATE_RE.match(str(record["verified_at"])):
        errors.append("verified_at must be ISO-8601")
    return errors


def load():
    if not RECORD.is_file():
        raise SystemExit(f"Missing compatibility record: {RECORD}")
    return json.loads(RECORD.read_text(encoding="utf-8"))


def save(record):
    errors = validate(record)
    if errors:
        raise SystemExit("Invalid compatibility record:\n- " + "\n- ".join(errors))
    RECORD.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--status", choices=sorted(STATUSES))
    parser.add_argument("--verified-at", default="")
    parser.add_argument("--verified-by", default="")
    parser.add_argument("--note", default="")
    parser.add_argument("--artifact", action="append", default=[], help="JSON object for one artifact")
    parser.add_argument("--print", action="store_true", dest="print_record")
    args = parser.parse_args()

    record = load()
    if args.status or args.verified_at or args.verified_by or args.note or args.artifact:
        if args.status:
            record["status"] = args.status
        if args.verified_at:
            record["verified_at"] = args.verified_at
        elif args.status == "verified" and not record.get("verified_at"):
            record["verified_at"] = datetime.now(timezone.utc).isoformat()
        if args.verified_by:
            record["verified_by"] = args.verified_by
        if args.note:
            record["notes"] = args.note
        record["patchset_revision"] = patchset_revision()
        if args.artifact:
            record["artifacts"] = [json.loads(item) for item in args.artifact]
        save(record)
        print(f"Updated {RECORD}")
    errors = validate(record)
    if errors:
        raise SystemExit("Invalid compatibility record:\n- " + "\n- ".join(errors))
    print(f"OK {record['id']} status={record['status']} patchset={record['patchset_revision']}")
    if args.print_record:
        print(json.dumps(record, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
