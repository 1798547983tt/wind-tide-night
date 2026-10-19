#!/usr/bin/env python3
"""Pack the AIS cookie helper extension into web/public/tools/ais-cookie-helper.zip.

wind-tide: the zip is what the donation page offers for download
(wind-tide/API.md §15); rerun this after editing anything under
wind-tide/tools/ais-cookie-helper/. The archive is deterministic (fixed
timestamps, sorted entries) so a rebuild without changes leaves git clean.
"""

from __future__ import annotations

import pathlib
import sys
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "wind-tide" / "tools" / "ais-cookie-helper"
TARGET = ROOT / "web" / "public" / "tools" / "ais-cookie-helper.zip"
FIXED_TIME = (2026, 1, 1, 0, 0, 0)


def main() -> int:
    files = sorted(path for path in SOURCE.rglob("*") if path.is_file())
    if not files:
        print(f"nothing to pack under {SOURCE}", file=sys.stderr)
        return 1
    TARGET.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(TARGET, "w") as archive:
        for path in files:
            name = "ais-cookie-helper/" + path.relative_to(SOURCE).as_posix()
            info = zipfile.ZipInfo(name, date_time=FIXED_TIME)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            archive.writestr(info, path.read_bytes())
    print(f"{TARGET.relative_to(ROOT).as_posix()}: {len(files)} files, {TARGET.stat().st_size} bytes")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
