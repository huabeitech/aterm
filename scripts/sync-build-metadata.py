#!/usr/bin/env python3
import argparse
import json
import plistlib
import subprocess
from pathlib import Path
from build_metadata import parse_info, require


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Sync Wails v3 build metadata from build/config.yml into Info.plist."
    )
    parser.add_argument("--config", default="build/config.yml")
    parser.add_argument("--plist", action="append", default=[])
    parser.add_argument("--print-field", help="Print one info field without writing files.")
    parser.add_argument("--runtime-json", help="Generate the embedded runtime product metadata.")
    args = parser.parse_args()

    info = parse_info(Path(args.config))
    if args.print_field:
        print(require(info, args.print_field))
        return
    if not args.plist and not args.runtime_json:
        parser.error("at least one --plist or --runtime-json destination is required")
    product_name = require(info, "productName")
    product_identifier = require(info, "productIdentifier")
    version = require(info, "version")
    build_number = require(info, "buildNumber")

    if args.runtime_json:
        runtime = {
            "name": product_name,
            "version": version,
            "buildNumber": build_number,
            "identifier": product_identifier,
            "description": info.get("description", "").strip(),
        }
        Path(args.runtime_json).write_text(json.dumps(runtime, indent=2) + "\n", encoding="utf-8")

    for plist_arg in args.plist:
        plist_path = Path(plist_arg)
        subprocess.run(["plutil", "-convert", "xml1", str(plist_path)], check=True)
        with plist_path.open("rb") as plist_file:
            plist = plistlib.load(plist_file)

        plist["CFBundleName"] = product_name
        plist["CFBundleExecutable"] = product_name
        plist["CFBundleIdentifier"] = product_identifier
        plist["CFBundleShortVersionString"] = version
        plist["CFBundleVersion"] = build_number
        plist["LSMinimumSystemVersion"] = require(info, "macOSMinimumVersion")

        description = info.get("description", "").strip()
        if description:
            plist["CFBundleGetInfoString"] = description

        copyright_text = info.get("copyright", "").strip()
        if copyright_text:
            plist["NSHumanReadableCopyright"] = copyright_text

        with plist_path.open("wb") as plist_file:
            plistlib.dump(plist, plist_file, sort_keys=False)


if __name__ == "__main__":
    main()
