#!/usr/bin/env python3

import argparse
import re
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Promote CHANGELOG Unreleased section into a versioned release section."
    )
    parser.add_argument("--changelog", default="CHANGELOG.md")
    parser.add_argument("--version", required=True)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--base", required=True)
    parser.add_argument("--new-entries-file", default="")
    return parser.parse_args()


def load_optional_file(path: str) -> str:
    if not path:
        return ""
    file_path = Path(path)
    if not file_path.exists():
        return ""
    return file_path.read_text(encoding="utf-8").strip()


def main() -> int:
    args = parse_args()

    changelog_path = Path(args.changelog)
    changelog_text = changelog_path.read_text(encoding="utf-8")

    pattern = r"## \[Unreleased\](.*?)(?=\n## \[|\Z)"
    match = re.search(pattern, changelog_text, re.DOTALL)
    if not match:
        raise RuntimeError("CHANGELOG.md is missing the '## [Unreleased]' section")

    unreleased_body = match.group(1).strip()
    new_entries = load_optional_file(args.new_entries_file)

    chunks = [part for part in [unreleased_body, new_entries] if part]
    body = "\n\n".join(chunks).strip() if chunks else ""
    if not body:
        body = f"- Release {args.version}"

    release_url = f"https://github.com/{args.repo}/releases/tag/{args.version}"
    compare_url = f"https://github.com/{args.repo}/compare/{args.base}...{args.version}"
    version_header = f"## [{args.version}]({release_url}) ([diff]({compare_url}))"

    replacement = f"## [Unreleased]\n\n{version_header}\n\n{body}\n"
    updated = changelog_text[: match.start()] + replacement + changelog_text[match.end() :]
    changelog_path.write_text(updated, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
