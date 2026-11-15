#!/usr/bin/env python3
"""Check actual bundle, Mach-O deployment target and embedded Go toolchain."""
import argparse
from pathlib import Path
import plistlib
import re
import subprocess
from build_metadata import parse_info, require


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("bundle", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    info = parse_info(root / "build/config.yml")
    plist = plistlib.loads((args.bundle / "Contents/Info.plist").read_bytes())
    expected = {
        "CFBundleName": require(info, "productName"),
        "CFBundleExecutable": require(info, "productName"),
        "CFBundleIdentifier": require(info, "productIdentifier"),
        "CFBundleShortVersionString": require(info, "version"),
        "CFBundleVersion": require(info, "buildNumber"),
        "LSMinimumSystemVersion": require(info, "macOSMinimumVersion"),
    }
    for key, value in expected.items():
        if plist.get(key) != value:
            raise SystemExit(f"Bundle {key}={plist.get(key)!r}; expected {value!r}")
    binary = args.bundle / "Contents/MacOS" / expected["CFBundleExecutable"]
    load_commands = subprocess.check_output(["xcrun", "vtool", "-show-build", str(binary)], text=True)
    minimums = re.findall(r"\bminos\s+(\d+(?:\.\d+)+)", load_commands)
    def normalize(value: str) -> tuple[int, ...]:
        return tuple(int(part) for part in (value.split(".") + ["0", "0"])[:3])
    target = expected["LSMinimumSystemVersion"]
    if not minimums or any(normalize(value) != normalize(target) for value in minimums):
        raise SystemExit(f"Mach-O minimums {minimums!r} do not match macOS {target}")
    build = subprocess.check_output(["go", "version", "-m", str(binary)], text=True)
    version = re.search(r"\bgo1\.\d+\.\d+\b", build.splitlines()[0])
    toolchain = require(info, "macOSGoToolchain")
    if not version or version[0] != toolchain:
        raise SystemExit(f"Binary Go toolchain differs from configured {toolchain}")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(args.bundle)], check=True)
    print(f"Verified {expected['CFBundleName']} {expected['CFBundleShortVersionString']}: macOS {target}, {toolchain}")


if __name__ == "__main__":
    main()
