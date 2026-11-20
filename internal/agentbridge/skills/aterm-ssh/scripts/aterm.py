"""Query the aTerm instance associated with this installed skill."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time


def socket_ready(path):
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(0.2)
        try:
            connection.connect(str(path))
            return True
        except (FileNotFoundError, ConnectionRefusedError, TimeoutError):
            return False


def ensure_running(path, profile):
    if socket_ready(path):
        return
    try:
        config = json.loads((path.parent / "agent-access.json").read_text())
    except FileNotFoundError:
        sys.exit("AGENT_DISABLED")
    if config.get("enabled") is not True:
        sys.exit("AGENT_DISABLED")
    bundle = profile.get("bundle")
    if not bundle or not Path(bundle).is_dir():
        sys.exit("APP_UNAVAILABLE")

    # Serialize cold starts so parallel calls do not reopen the app.
    deadline = time.monotonic() + 15
    fd = os.open(path.parent / "agent-launch.lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        while True:
            try:
                fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                if time.monotonic() >= deadline:
                    sys.exit("APP_START_TIMEOUT")
                time.sleep(0.1)
        if socket_ready(path):
            return
        command = ["/usr/bin/open", "-g", "-j", "-a", bundle, "--args", "--agent-background"]
        if profile.get("dataDir"):
            command.append(profile["dataDir"])
        try:
            subprocess.run(command, check=True, timeout=5, capture_output=True)
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
            sys.exit("APP_UNAVAILABLE")
        while time.monotonic() < deadline:
            if socket_ready(path):
                return
            time.sleep(0.1)
        sys.exit("APP_START_TIMEOUT")
    finally:
        os.close(fd)


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="action", required=True)
    commands.add_parser("list").add_argument("query", nargs="?", default="")
    commands.add_parser("prepare").add_argument("host_id")
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent.parent
    profile_path = directory / "profile.json"
    profile = json.loads(profile_path.read_text()) if profile_path.is_file() else {}
    socket_path = Path(profile.get("socket", Path.home() / "Library/Application Support/aTerm/agent.sock"))
    ensure_running(socket_path, profile)
    request = {"Method": args.action}
    if args.action == "prepare":
        request["HostID"] = args.host_id
    else:
        request["Query"] = args.query
    response = subprocess.run(
        ["/usr/bin/curl", "--disable", "--silent", "--show-error", "--fail",
         "--noproxy", "*", "--max-time", "100", "--unix-socket", str(socket_path),
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
