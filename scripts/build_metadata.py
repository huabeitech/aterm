"""Read the flat info section shared by product/build metadata tools."""
import re
from pathlib import Path


def parse_info(config_path: Path) -> dict[str, str]:
    info: dict[str, str] = {}
    in_info = False

    for raw_line in config_path.read_text(encoding="utf-8").splitlines():
        if not raw_line.strip() or raw_line.lstrip().startswith("#"):
            continue

        if not raw_line.startswith((" ", "\t")):
            in_info = raw_line.strip() == "info:"
            continue

        if not in_info:
            continue

        match = re.match(r"\s{2,}([A-Za-z0-9_]+):\s*(.*)\s*$", raw_line)
        if not match:
            continue

        key, value = match.groups()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
            value = value[1:-1]
        info[key] = value

    return info


def require(info: dict[str, str], key: str) -> str:
    value = info.get(key, "").strip()
    if not value:
        raise SystemExit(f"missing info.{key} in build config")
    return value


