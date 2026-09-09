"""Fail even when Gradle reports success after an empty/crashed device test run."""
from pathlib import Path
import sys
import xml.etree.ElementTree as ET


def check(directory):
    reports = list(Path(directory).glob("TEST*.xml"))
    total = failed = skipped = 0
    for report in reports:
        root = ET.parse(report).getroot()
        total += int(root.get("tests", "0"))
        failed += int(root.get("failures", "0")) + int(root.get("errors", "0"))
        skipped += int(root.get("skipped", "0"))
    if not reports or total <= skipped or failed:
        raise SystemExit(f"Device tests incomplete: {total} tests, {failed} failures/errors, {skipped} skipped")
    print(f"Device results verified: {total} tests, {skipped} skipped", flush=True)


if __name__ == "__main__":
    check(sys.argv[1])
