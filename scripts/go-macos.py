#!/usr/bin/env python3
"""Run Go with the reproducible macOS-12-compatible shipping toolchain."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from build_metadata import parse_info, require


def main() -> None:
    if sys.platform != "darwin":
        raise SystemExit("The macOS build helper requires a macOS build host.")
    if len(sys.argv) < 2:
        raise SystemExit("Usage: python3 scripts/go-macos.py <go command> [arguments]")
    root = Path(__file__).resolve().parent.parent
    info = parse_info(root / "build/config.yml")
    target = require(info, "macOSMinimumVersion")
    toolchain = require(info, "macOSGoToolchain")
    if not re.fullmatch(r"\d+\.\d+(?:\.\d+)?", target):
        raise SystemExit("Invalid macOS minimum version")
    version = re.fullmatch(r"go1\.(\d+)\.\d+", toolchain)
    if not version:
        raise SystemExit("Pin an exact stable Go toolchain for macOS builds")
    if int(target.split(".")[0]) < 13 and int(version[1]) >= 27:
        raise SystemExit("Go 1.27+ cannot build an application supporting macOS 12")
    environment = os.environ.copy()
    environment["GOTOOLCHAIN"] = toolchain  # Exact: dependency changes cannot silently upgrade it.
    environment["CGO_ENABLED"] = "1"
    environment["MACOSX_DEPLOYMENT_TARGET"] = target
    names = ["CGO_CFLAGS", "CGO_CXXFLAGS", "CGO_LDFLAGS"]
    flags = json.loads(subprocess.check_output(["go", "env", "-json", *names], env=environment))
    for name in names:
        environment[name] = flags[name] + f" -mmacosx-version-min={target}"
    command = sys.argv[1:]
    if command[0] == "--exec":
        command = command[1:]
        if not command:
            raise SystemExit("--exec requires a command")
    else:
        command = ["go", *command]
    os.execvpe(command[0], command, environment)


if __name__ == "__main__":
    main()
