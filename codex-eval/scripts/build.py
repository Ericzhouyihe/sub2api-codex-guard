#!/usr/bin/env python3
"""Download Sub2API, apply all plugin patches, run tests, and build the installer."""

import argparse
from datetime import datetime, timezone
import gzip
import hashlib
import json
import os
import platform
from pathlib import Path, PurePosixPath
import re
import runpy
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
UPSTREAM = os.environ.get("SUB2API_UPSTREAM", "https://github.com/Wei-Shaw/sub2api.git")
PNPM = os.environ.get("SUB2API_PNPM", "pnpm@12.16.1")
PNPM_BIN = os.environ.get("SUB2API_PNPM_BIN", "")
DEFAULT_UPSTREAM_TAG = "v0.2.21"
TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"), ("darwin", "arm64"), ("windows", "amd64"))
PTERM = "github.com/pterm/pterm@v0.12.84"


def resolve_upstream_tag():
    tag = os.environ.get("UPSTREAM_TAG", DEFAULT_UPSTREAM_TAG).strip()
    if not tag:
        raise RuntimeError("UPSTREAM_TAG must not be empty; use the default v0.2.21 or an explicit tag.")
    if not isinstance(tag, str) or not re.fullmatch(r"v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?", tag):
        raise RuntimeError(f"Invalid upstream version tag: {tag!r}")
    return tag if tag.startswith("v") else f"v{tag}"


def write_build_status(root, state, tag="", detail="", artifacts=None):
    path = root / "dist" / "build-status.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "state": state,
        "tag": tag,
        "detail": detail,
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }
    if artifacts:
        payload["artifacts"] = [str(item) for item in artifacts]
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def mark_build_failure(root, tag, error):
    write_build_status(root, "failed", tag, str(error))
    marker = root / "dist" / "BUILD_FAILED.txt"
    marker.write_text(
        "This build failed. Do not treat dist/build as the output of this run.\n"
        f"tag: {tag or '<unresolved>'}\nerror: {error}\n",
        encoding="utf-8",
    )


def archive_previous_output(root):
    """Move a previous dist/build aside so a failed run cannot masquerade as success."""
    output = root / "dist" / "build"
    if not output.exists():
        return None
    stale_root = root / "dist" / ".stale"
    stale_root.mkdir(parents=True, exist_ok=True)
    target = stale_root / ("build-" + datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S"))
    suffix = 1
    while target.exists():
        target = stale_root / ("build-" + datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S") + f"-{suffix}")
        suffix += 1
    shutil.move(str(output), str(target))
    return target


def require_version(name, argv, minimum, parser):
    binary = shutil.which(argv[0])
    if not binary:
        raise RuntimeError(f"{name} is required but was not found on PATH. See docs/codex-eval-fork.md.")
    result = subprocess.run([binary, *argv[1:]], text=True, capture_output=True)
    if result.returncode != 0:
        raise RuntimeError(f"{name} version check failed: {result.stderr.strip() or result.stdout.strip()}")
    version = parser(result.stdout + result.stderr)
    if version is None:
        raise RuntimeError(f"{name} version could not be parsed: {result.stdout.strip() or result.stderr.strip()}")
    if version < minimum:
        raise RuntimeError(f"{name} {version} is too old; need >= {minimum}")
    return version


def check_prerequisites():
    if sys.version_info < (3, 11):
        raise RuntimeError("Python >= 3.11 is required.")
    require_version("Go", ["go", "version"], (1, 27), lambda text: _go_version(text))
    require_version("Node", ["node", "--version"], (20, 0), lambda text: _node_version(text))
    if PNPM_BIN:
        if not Path(PNPM_BIN).is_file():
            raise RuntimeError(f"SUB2API_PNPM_BIN does not exist: {PNPM_BIN}")
    elif not shutil.which("corepack"):
        raise RuntimeError("corepack is required when SUB2API_PNPM_BIN is not set.")


def _go_version(text):
    match = re.search(r"go(\d+)\.(\d+)", text)
    return (int(match.group(1)), int(match.group(2))) if match else None


def _node_version(text):
    match = re.search(r"v?(\d+)\.(\d+)", text)
    return (int(match.group(1)), int(match.group(2))) if match else None


def artifact_name(system, arch, tag, installer=False):
    kind = "-installer" if installer else ""
    suffix = ".exe" if system == "windows" else ""
    return f"sub2api-plugins{kind}-{tag}-{system}-{arch}{suffix}"


def run(argv, cwd, env=None):
    print("+ " + " ".join(map(str, argv)), flush=True)
    subprocess.run(list(map(str, argv)), cwd=cwd, env=env, check=True)


def pnpm(argv, cwd, env=None):
    """Run pnpm through SUB2API_PNPM_BIN when provided, otherwise corepack."""
    if PNPM_BIN:
        node = os.environ.get("SUB2API_NODE", "node")
        return run([node, PNPM_BIN, *argv], cwd, env)
    return run(["corepack", PNPM, *argv], cwd, env)


def discover_plugins(root):
    if (root / "patches").is_dir():
        return [root]
    modules = root / ".gitmodules"
    if not modules.is_file():
        return []
    result = subprocess.run(
        ["git", "config", "--null", "--file", str(modules), "--get-regexp", r"^submodule\..*\.path$"],
        cwd=root, text=True, capture_output=True,
    )
    if result.returncode not in (0, 1):
        raise RuntimeError(result.stderr.strip())
    paths = [entry.split("\n", 1)[1] for entry in result.stdout.split("\0") if entry]
    plugins = sorted(path for path in paths if PurePosixPath(path).parts[0] == "plugins")
    for path in plugins:
        if not (root / path / ".git").exists():
            raise RuntimeError(f"Uninitialized submodule {path}. Run: git submodule update --init --recursive -- {path}")
    return [root / path for path in plugins if any((root / path / "patches").glob("*.patch"))]


def apply_patches(tree, plugins):
    for plugin in plugins:
        for patch in sorted((plugin / "patches").glob("*.patch")):
            data = patch.read_bytes()
            temporary = None
            if b"\r\n" in data:
                handle, name = tempfile.mkstemp(suffix=".patch", prefix="normalized-")
                with os.fdopen(handle, "wb") as output:
                    output.write(data.replace(b"\r\n", b"\n"))
                temporary = Path(name)
            try:
                run(["git", "apply", "--binary", str(temporary or patch)], tree)
            except subprocess.CalledProcessError as error:
                raise RuntimeError(f"Patch failed: {plugin.name}/{patch.name}") from error
            finally:
                if temporary is not None:
                    temporary.unlink(missing_ok=True)


def test_entries(owner, tree, env, installer=None, common=None):
    support = runpy.run_path(str(ROOT / "tests/support.py"))
    support["test_entries"](owner, tree, env, run, installer, common)


def collect_licenses(staging, root, plugins):
    destination = staging / "licenses"
    destination.mkdir()
    for owner in [root, *(plugin for plugin in plugins if plugin != root)]:
        namespace = Path() if owner == root else owner.relative_to(root / "plugins")
        target = destination / namespace
        target.mkdir(parents=True, exist_ok=True)
        if (owner / "LICENSE").is_file():
            shutil.copyfile(owner / "LICENSE", target / "LICENSE")
        if (owner / "licenses").is_dir():
            shutil.copytree(owner / "licenses", target, dirs_exist_ok=True)


def prepare_installer(workspace, tree, root, plugins, env):
    staging = workspace / "installer"
    if staging.exists():
        shutil.rmtree(staging)
    shutil.copytree(root / "scripts/installer", staging)
    lines = (tree / "backend/go.mod").read_text().splitlines()
    directives = [line for line in lines if line.startswith(("go ", "toolchain "))]
    (staging / "go.mod").write_text("module sub2api-plugins-installer\n\n" + "\n".join(directives) + "\n")
    dependency_env = dict(env, GOFLAGS="-mod=mod")
    run(["go", "get", PTERM], staging, dependency_env)
    run(["go", "mod", "tidy"], staging, dependency_env)
    collect_licenses(staging, root, plugins)
    modules = subprocess.run(["go", "list", "-m", "-json", "all"], cwd=staging, env=env, text=True, capture_output=True, check=True)
    decoder = json.JSONDecoder()
    remaining = modules.stdout.lstrip()
    while remaining:
        module, end = decoder.raw_decode(remaining)
        remaining = remaining[end:].lstrip()
        if module.get("Main") or "Dir" not in module:
            continue
        directory = Path(module["Dir"])
        notices = [path for path in directory.iterdir() if path.is_file() and path.name.upper().startswith(("LICENSE", "COPYING", "NOTICE"))]
        for notice in notices:
            destination = staging / "licenses/installer-dependencies" / module["Path"] / notice.name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(notice, destination)
    return staging


def publish_outputs(prepared, output):
    output.parent.mkdir(parents=True, exist_ok=True)
    publishing = Path(tempfile.mkdtemp(prefix=".sub2api-plugins-", dir=output.parent))
    candidate, previous = publishing / "candidate", publishing / "previous"
    try:
        # Complete any cross-filesystem copy before moving the existing output directory.
        shutil.copytree(prepared, candidate)
        if output.exists():
            output.rename(previous)
        try:
            candidate.rename(output)
        except OSError:
            if previous.exists():
                try:
                    previous.rename(output)
                except OSError as error:
                    raise RuntimeError(f"Restore previous outputs from {previous}: {error}") from error
            raise
    finally:
        if not previous.exists() or output.exists():
            shutil.rmtree(publishing)


def selected_platforms(value=None):
    supported = [f"{system}-{arch}" for system, arch in TARGETS]
    if value is None:
        return supported
    selected = value.split()
    if not selected or any(target not in supported for target in selected):
        raise RuntimeError("Select at least one supported build platform.")
    return [target for target in supported if target in selected]


def common_targets():
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[platform.machine().lower()]
    return sorted({"linux-amd64", f"linux-{arch}"})


def required_servers(selected):
    required = set(common_targets())
    for target in selected:
        required.update((target, "linux-" + target.split("-")[1]))
    return sorted(required)


def build_environment(tag):
    env = os.environ.copy()
    for name in ("GOOS", "GOARCH", "CGO_ENABLED"):
        env.pop(name, None)
    env.update(GOFLAGS=os.environ.get("SUB2API_GOFLAGS", "-mod=readonly"), PYTHONDONTWRITEBYTECODE="1", SUB2API_BUILD_VERSION=tag.removeprefix("v"), SUB2API_PNPM=PNPM)
    env.setdefault("COREPACK_ENABLE_DOWNLOAD_PROMPT", "0")
    env.setdefault("COREPACK_ENABLE_AUTO_PIN", "0")
    env["COREPACK_ENABLE_PROJECT_SPEC"] = "0"
    return env


def source(root, workspace, tag):
    plugins = discover_plugins(root)
    tree = workspace / "source"
    run(["git", "clone", "--depth", "1", "--branch", tag, UPSTREAM, tree], workspace)
    run(["git", "checkout", "--detach", f"refs/tags/{tag}"], tree)
    apply_patches(tree, plugins)


def server(workspace, tag, target):
    system, arch = target.split("-")
    env = dict(build_environment(tag), GOOS=system, GOARCH=arch, CGO_ENABLED="0")
    binary = workspace / "binaries" / artifact_name(system, arch, tag)
    binary.parent.mkdir(exist_ok=True)
    run(["go", "build", "-tags=embed", "-trimpath", f"-ldflags=-s -w -X main.Version={tag.removeprefix('v')} -X main.BuildType=release", "-o", binary, "./cmd/server"], workspace / "source/backend", env)
    return binary


def embed_payloads(workspace, staging, tag, targets):
    payloads = staging / "payloads"
    if payloads.exists():
        shutil.rmtree(payloads)
    payloads.mkdir()
    for target in sorted(set(targets)):
        system, arch = target.split("-")
        binary = workspace / "binaries" / artifact_name(system, arch, tag)
        with binary.open("rb") as original, gzip.open(payloads / (target + ".gz"), "wb") as compressed:
            shutil.copyfileobj(original, compressed)


def prepare(root, workspace, tag):
    tree = workspace / "source"
    plugins = discover_plugins(root)
    env = build_environment(tag)
    frontend = tree / "frontend"
    settings = json.loads((frontend / "package.json").read_text()).get("pnpm", {})
    # pnpm 12 moved overrides and build settings out of package.json and .npmrc.
    # Keep upstream pnpm 9's dependency-script behavior without changing its lockfile.
    settings.update(ignoreScripts=False, dangerouslyAllowAllBuilds=True, strictPeerDependencies=False)
    (frontend / "pnpm-workspace.yaml").write_text(json.dumps(settings, indent=2) + "\n")
    if not PNPM_BIN:
        run(["corepack", "prepare", PNPM, "--activate"], root, env)
    if os.environ.get("SUB2API_SKIP_FRONTEND") != "1":
        install_args = os.environ.get("SUB2API_PNPM_INSTALL_ARGS", "--frozen-lockfile").split()
        pnpm(["install", *install_args], tree / "frontend", env)
        pnpm(["build"], tree / "frontend", env)
    if os.environ.get("SUB2API_SKIP_TESTS") != "1":
        for plugin in plugins:
            test_entries(plugin, tree, env, common=False)
        run(["go", "vet", "./internal/server", "./internal/server/routes", "./internal/web"], tree / "backend", env)
    staging = prepare_installer(workspace, tree, root, plugins, env)
    for target in common_targets():
        server(workspace, tag, target)
    embed_payloads(workspace, staging, tag, common_targets())
    if os.environ.get("SUB2API_SKIP_TESTS") != "1":
        test_entries(root, tree, env, staging, common=True if root in plugins else None)


def installer(workspace, tag, target):
    system, arch = target.split("-")
    staging = workspace / "installer"
    embed_payloads(workspace, staging, tag, (target, f"linux-{arch}"))
    binary = workspace / "installers" / artifact_name(system, arch, tag, installer=True)
    binary.parent.mkdir(exist_ok=True)
    env = dict(build_environment(tag), GOOS=system, GOARCH=arch, CGO_ENABLED="0")
    run(["go", "build", "-trimpath", f"-ldflags=-s -w -X main.buildVersion={tag.removeprefix('v')}", "-o", binary, "."], staging, env)
    return binary


def write_checksums(directory):
    lines = []
    for path in sorted(item for item in directory.iterdir() if item.is_file()):
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        lines.append(f"{digest}  {path.name}")
    (directory / "SHA256SUMS").write_text("\n".join(lines) + "\n", encoding="utf-8")


def publish(root, workspace, tag, selected):
    prepared = workspace / "outputs"
    prepared.mkdir()
    for target in selected:
        system, arch = target.split("-")
        directory = prepared / target
        directory.mkdir()
        for kind, is_installer in (("binaries", False), ("installers", True)):
            name = artifact_name(system, arch, tag, installer=is_installer)
            binary = directory / name
            shutil.copyfile(workspace / kind / name, binary)
            binary.chmod(0o755)
        write_checksums(directory)
    output = root / "dist/build"
    publish_outputs(prepared, output)
    return sorted(path for path in output.glob("*/*") if path.is_file())


def build(root, workspace, tag, selected=None):
    selected = selected if selected is not None else selected_platforms()
    source(root, workspace, tag)
    return resume(root, workspace, tag, selected)


def resume(root, workspace, tag, selected):
    archive_previous_output(root)
    prepare(root, workspace, tag)
    for target in sorted(set(required_servers(selected)) - set(common_targets())):
        server(workspace, tag, target)
    for target in selected:
        installer(workspace, tag, target)
    return publish(root, workspace, tag, selected)


def stage_outputs(tag, selected):
    outputs = {
        "tag": tag,
        "pnpm": PNPM,
        "platforms": " ".join(selected),
    }
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            for name, value in outputs.items():
                print(f"{name}={value}", file=output)


def run_stage(root, workspace, stage, tag, selected):
    if stage == "source":
        source(root, workspace, tag)
        stage_outputs(tag, selected)
    elif stage == "resume":
        return resume(root, workspace, tag, selected)
    else:
        raise RuntimeError(f"Unknown build stage: {stage}")
    return []


def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    tag = ""
    (ROOT / "dist" / "BUILD_FAILED.txt").unlink(missing_ok=True)
    try:
        stage = os.environ.get("SUB2API_BUILD_STAGE")
        selected = selected_platforms(os.environ.get("SUB2API_BUILD_PLATFORMS"))
        tag = resolve_upstream_tag() if not stage or stage == "source" else os.environ.get("UPSTREAM_TAG", DEFAULT_UPSTREAM_TAG)
        if not tag:
            raise RuntimeError("Pass the resolved UPSTREAM_TAG to the resumed build stage.")
        if stage != "source":
            check_prerequisites()
        write_build_status(ROOT, "building", tag, "Build started")
        if stage:
            location = os.environ.get("SUB2API_BUILD_WORKSPACE")
            if not location:
                raise RuntimeError("SUB2API_BUILD_WORKSPACE is required for a resumed build stage.")
            workspace = Path(location).resolve()
            workspace.mkdir(parents=True, exist_ok=True)
            outputs = run_stage(ROOT, workspace, stage, tag, selected)
        else:
            with tempfile.TemporaryDirectory(prefix="sub2api-plugins-") as temporary:
                print(f"Build workspace: {temporary}", flush=True)
                outputs = build(ROOT, Path(temporary), tag, selected)
        write_build_status(ROOT, "success", tag, "Build completed", outputs)
        for output in outputs:
            print(output, flush=True)
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        mark_build_failure(ROOT, tag, error)
        print(f"Build failed: {error}. No new outputs were published.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
