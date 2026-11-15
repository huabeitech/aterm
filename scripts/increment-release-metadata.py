#!/usr/bin/env python3
import argparse
import re
from pathlib import Path


def replace_numeric_field(text: str, field_name: str) -> tuple[str, int]:
    pattern = re.compile(rf'^(\s*{re.escape(field_name)}:\s*["\']?)(\d+)(["\']?\s*)$', re.MULTILINE)
    match = pattern.search(text)
    if not match:
        raise SystemExit(f"missing numeric info.{field_name}")

    next_value = int(match.group(2)) + 1
    updated = text[: match.start()] + f"{match.group(1)}{next_value}{match.group(3)}" + text[match.end():]
    return updated, next_value


def replace_patch_version(text: str) -> tuple[str, str]:
    pattern = re.compile(r'^(\s*version:\s*["\']?)(\d+)\.(\d+)\.(\d+)(["\']?\s*)$', re.MULTILINE)
    match = pattern.search(text)
    if not match:
        raise SystemExit("missing semantic info.version in x.y.z format")

    major = match.group(2)
    minor = match.group(3)
    patch = int(match.group(4)) + 1
    next_version = f"{major}.{minor}.{patch}"
    updated = text[: match.start()] + f"{match.group(1)}{next_version}{match.group(5)}" + text[match.end():]
    return updated, next_version


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Increment info.version patch and info.buildNumber in the Wails build config."
    )
    parser.add_argument("--config", default="build/config.yml")
    args = parser.parse_args()

    config_path = Path(args.config)
    text = config_path.read_text(encoding="utf-8")
    text, next_version = replace_patch_version(text)
    text, next_build = replace_numeric_field(text, "buildNumber")
    config_path.write_text(text, encoding="utf-8")
    print(f"{next_version} {next_build}")


if __name__ == "__main__":
    main()
