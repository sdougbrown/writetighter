"""Deterministic corpus snapshot flow.

The eval corpus is *not* committed: it is copied (and sha256-hashed) from
public repository checkouts at setup time by ``evals/scripts/snapshot_corpus.py``
and gitignored. This module holds the selection/copy/check logic so it is unit
testable and reusable.

Manifest schema (see ``evals/scripts/corpus_manifest.yaml``):

    repo_root: <str>    # dir (relative to the manifest file) that *path* values resolve against
    output: <str>       # dir (relative to repo_root) to materialise into
    lockfile: <str>     # generated hash lock (relative to repo_root); committed metadata, not corpus
    repos:
      - id: <str>
        path: <str>     # repo dir relative to repo_root
        url: <str>      # canonical public clone URL
        revision: <str> # full commit containing the locked source
        language: <str> # go | ts | rust | py
        files: [<rel path>, ...]
        exclude_prefixes: [<rel dir or filename>]  # defensive; pinned files are explicit

Selection is pinned (explicit file lists) so the eval is reproducible even as
source checkouts drift; the lockfile records the exact sha256 of each ingested
file and ``--check`` fails when a source file has changed under the eval's feet.
"""

from __future__ import annotations

import hashlib
import json
import shutil
from dataclasses import dataclass, field
from pathlib import Path

import yaml

# Defensive excludes applied to every repo whether or not a pinned file matches.
_EXCLUDE_DIR_PARTS = {
    ".git",
    "node_modules",
    ".venv",
    "vendor",
    "target",
    "dist",
    "build",
    "bin",
    "obj",
    "__pycache__",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
    ".next",
    "cache",
}
_EXCLUDE_FILE_PARTS = {
    "package-lock.json",
    "yarn.lock",
    "pnpm-lock.yaml",
    "uv.lock",
    "Cargo.lock",
    "go.sum",
}


@dataclass(frozen=True)
class ManifestEntry:
    repo_id: str
    language: str
    rel_path: str


@dataclass
class Corpus:
    """Loaded manifest plus resolved repo roots and output/lockfile paths."""

    manifest: dict
    repo_root: Path
    output: Path
    lockfile: Path
    entries: list[ManifestEntry] = field(default_factory=list)

    @classmethod
    def load(cls, manifest_path: str | Path) -> Corpus:
        p = Path(manifest_path).resolve()
        data = yaml.safe_load(p.read_text()) or {}
        # repo_root is relative to the manifest file's directory.
        repo_root = (p.parent / data.get("repo_root", ".")).resolve()
        output = (
            (repo_root / data["output"]).resolve()
            if "output" in data
            else repo_root / "corpus"
        )
        lockfile = (
            (repo_root / data["lockfile"]).resolve()
            if "lockfile" in data
            else output.with_suffix(".lock.json")
        )
        entries: list[ManifestEntry] = []
        for repo in data.get("repos", []):
            _validate_repository_source(repo)
            lang = repo["language"]
            repo_dir = (repo_root / repo["path"]).resolve()
            excludes = _exclude_prefixes(repo)
            for rel in repo.get("files", []):
                if _defensively_excluded(rel, excludes):
                    continue
                if not (repo_dir / rel).is_file():
                    raise FileNotFoundError(
                        f"corpus source missing: {repo_dir / rel}. "
                        f"Clone {repo['url']} into {repo_dir} and check out "
                        f"revision {repo['revision']}, or override path in a manifest copy."
                    )
                entries.append(
                    ManifestEntry(repo_id=repo["id"], language=lang, rel_path=rel)
                )
        # Deterministic ordering regardless of manifest order.
        entries.sort(key=lambda e: (e.language, e.repo_id, e.rel_path))
        return cls(
            manifest=data,
            repo_root=repo_root,
            output=output,
            lockfile=lockfile,
            entries=entries,
        )

    def dest_for(self, entry: ManifestEntry) -> Path:
        return self.output / entry.language / entry.repo_id / entry.rel_path


def _validate_repository_source(repo: dict) -> None:
    repo_id = repo.get("id", "<unknown>")
    url = repo.get("url")
    revision = repo.get("revision")
    if not isinstance(url, str) or not url.startswith("https://github.com/"):
        raise ValueError(f"corpus repository {repo_id!r} needs a public GitHub url")
    if (
        not isinstance(revision, str)
        or len(revision) != 40
        or any(char not in "0123456789abcdef" for char in revision.lower())
    ):
        raise ValueError(f"corpus repository {repo_id!r} needs a full commit revision")


def _exclude_prefixes(repo: dict) -> set[str]:
    return set(repo.get("exclude_prefixes", []))


def _defensively_excluded(rel: str, excludes: set[str]) -> bool:
    parts = Path(rel).parts
    if any(part in _EXCLUDE_DIR_PARTS for part in parts):
        return True
    if Path(rel).name in _EXCLUDE_FILE_PARTS:
        return True
    for prefix in excludes:
        if rel == prefix or rel.startswith(prefix.rstrip("/") + "/"):
            return True
    return False


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def snapshot(corpus: Corpus, *, force: bool = False) -> dict:
    """Copy pinned files into ``corpus.output`` and write the hash lockfile.

    Idempotent: when the lockfile matches every source file, files are left
    untouched unless *force* is set. Returns the lockfile payload.
    """
    if not force and corpus.lockfile.exists():
        lock = json.loads(corpus.lockfile.read_text())
        if _lock_matches(corpus, lock):
            return lock

    # The driver discovers files recursively, so stale files from a previous
    # manifest would silently expand the corpus unless rebuilds replace it.
    if corpus.output.exists():
        shutil.rmtree(corpus.output)

    records = []
    for entry in corpus.entries:
        src = corpus.repo_root / _entry_repo_path(corpus, entry) / entry.rel_path
        dest = corpus.dest_for(entry)
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dest)
        records.append(
            {
                "repo": entry.repo_id,
                "language": entry.language,
                "path": str(entry.rel_path),
                "sha256": sha256_file(src),
                "size": src.stat().st_size,
            }
        )

    lock = {
        "schema_version": 1,
        "output": str(_rel_to_root(corpus, corpus.output)),
        "files": sorted(records, key=lambda r: (r["language"], r["repo"], r["path"])),
    }
    corpus.lockfile.parent.mkdir(parents=True, exist_ok=True)
    corpus.lockfile.write_text(json.dumps(lock, indent=2) + "\n")
    return lock


def _entry_repo_path(corpus: Corpus, entry: ManifestEntry) -> Path:
    for repo in corpus.manifest.get("repos", []):
        if repo["id"] == entry.repo_id:
            return Path(repo["path"])
    raise KeyError(entry.repo_id)


def _rel_to_root(corpus: Corpus, p: Path) -> Path:
    return p.relative_to(corpus.repo_root)


def _lock_matches(corpus: Corpus, lock: dict) -> bool:
    by_key = {(r["language"], r["repo"], r["path"]): r for r in lock.get("files", [])}
    for entry in corpus.entries:
        src = corpus.repo_root / _entry_repo_path(corpus, entry) / entry.rel_path
        want = by_key.get((entry.language, entry.repo_id, entry.rel_path))
        if want is None or sha256_file(src) != want["sha256"]:
            return False
        dest = corpus.dest_for(entry)
        if not dest.exists():
            return False
    return True


def check(corpus: Corpus) -> tuple[list[dict], list[dict]]:
    """Compare current source checkouts to the committed lockfile.

    Returns (drifted, missing). Drifted entries have a changed sha256; missing
    entries are pinned in the manifest but have no lock record or no copy.
    """
    if not corpus.lockfile.exists():
        return [], [{"error": f"lockfile not found: {corpus.lockfile}"}]
    lock = json.loads(corpus.lockfile.read_text())
    by_key = {(r["language"], r["repo"], r["path"]): r for r in lock.get("files", [])}
    drifted: list[dict] = []
    missing: list[dict] = []
    for entry in corpus.entries:
        src = corpus.repo_root / _entry_repo_path(corpus, entry) / entry.rel_path
        rec = by_key.get((entry.language, entry.repo_id, entry.rel_path))
        if rec is None:
            missing.append(
                {
                    "repo": entry.repo_id,
                    "path": entry.rel_path,
                    "reason": "no lock record",
                }
            )
            continue
        if sha256_file(src) != rec["sha256"] or not corpus.dest_for(entry).exists():
            drifted.append(
                {
                    "repo": entry.repo_id,
                    "path": entry.rel_path,
                    "locked_sha256": rec["sha256"],
                    "current_sha256": sha256_file(src),
                }
            )
    return drifted, missing
