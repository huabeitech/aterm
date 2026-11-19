"""Query the aTerm instance associated with this installed skill."""
import argparse
import json
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="action", required=True)
    commands.add_parser("list").add_argument("query", nargs="?", default="")
    commands.add_parser("prepare").add_argument("host_id")
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent.parent
    profile = directory / "profile.json"
    if profile.is_file():
        socket = Path(json.loads(profile.read_text())["socket"])
    else:
        socket = Path.home() / "Library/Application Support/aTerm/agent.sock"
    request = {"Method": args.action}
    if args.action == "prepare":
        request["HostID"] = args.host_id
    else:
        request["Query"] = args.query
    response = subprocess.run(
        ["/usr/bin/curl", "--disable", "--silent", "--show-error", "--fail",
         "--noproxy", "*", "--max-time", "100", "--unix-socket", str(socket),
         "--data-binary", "@-", "http://localhost/request"],
        input=json.dumps(request), capture_output=True, text=True,
    )
    if response.returncode:
        sys.exit(response.stderr.strip())
    result = json.loads(response.stdout)
    if result.get("error"):
        sys.exit(result["error"])
    if args.action == "list":
        result.setdefault("hosts", [])
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
