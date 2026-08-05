"""Snapshot the eval corpus from public repo checkouts (sha256-locked).

This script materialises the *generated* corpus (copies of real source files
from repositories identified by public URL and commit in the manifest) into the
gitignored fixture seed and writes a hash lockfile. Local checkout paths remain
manifest-configurable.

Usage:
    python evals/scripts/snapshot_corpus.py                # copy + write lockfile
    python evals/scripts/snapshot_corpus.py --check        # verify vs lockfile
    python evals/scripts/snapshot_corpus.py --force        # force rebuild
    python evals/scripts/snapshot_corpus.py --dry-run      # print plan, change nothing

Run from anywhere; paths resolve against the writetighter repo root.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

_REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(_REPO_ROOT / "evals" / "src"))

from wt_evals import corpus as corpus_mod


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Snapshot the eval corpus from pinned public repo checkouts"
    )
    parser.add_argument(
        "--manifest", default=str(_REPO_ROOT / "evals/scripts/corpus_manifest.yaml")
    )
    parser.add_argument(
        "--check", action="store_true", help="verify current sources vs lockfile"
    )
    parser.add_argument(
        "--force", action="store_true", help="rebuild even if lockfile matches"
    )
    parser.add_argument(
        "--dry-run", action="store_true", help="print plan, don't write files"
    )
    parser.add_argument(
        "--sources",
        action="store_true",
        help="list canonical repository URLs, revisions, and expected local paths",
    )
    args = parser.parse_args()

    if args.sources:
        manifest_path = Path(args.manifest).resolve()
        data = yaml.safe_load(manifest_path.read_text()) or {}
        repo_root = (manifest_path.parent / data.get("repo_root", ".")).resolve()
        seen = set()
        for repo in data.get("repos", []):
            source = (repo["url"], repo["revision"], repo["path"])
            if source in seen:
                continue
            seen.add(source)
            print(
                f"{repo['url']} @ {repo['revision']} -> "
                f"{(repo_root / repo['path']).resolve()}"
            )
        return 0

    c = corpus_mod.Corpus.load(args.manifest)

    if args.check:
        drifted, missing = corpus_mod.check(c)
        if not drifted and not missing:
            print(f"corpus OK ({len(c.entries)} files) matches {c.lockfile.name}")
            return 0
        for d in drifted:
            print(
                f"DRIFT  {d['repo']}/{d['path']}: {d.get('locked_sha256', '?')} -> {d.get('current_sha256', '?')}"
            )
        for m in missing:
            print(
                f"MISS   {m.get('repo', '?')}/{m.get('path', '?')}: {m.get('error', m.get('reason'))}"
            )
        return 1

    if args.dry_run:
        for entry in c.entries:
            print(
                f"  {entry.language}  {entry.repo_id}/{entry.rel_path} -> {c.dest_for(entry)}"
            )
        print(f"plan: {len(c.entries)} files -> {c.output}")
        return 0

    lock = corpus_mod.snapshot(c, force=args.force)
    print(f"snapshotted {len(lock['files'])} files -> {c.output}")
    print(f"lockfile -> {c.lockfile}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
