import json
from pathlib import Path

import yaml
from wt_evals.corpus import Corpus, check, snapshot

REPO_ROOT = Path(__file__).resolve().parents[2]
MANIFEST = REPO_ROOT / "evals/scripts/corpus_manifest.yaml"


def test_manifest_selects_multilingual_existing_files() -> None:
    corpus = Corpus.load(MANIFEST)
    languages = {entry.language for entry in corpus.entries}
    assert languages == {"go", "ts", "rust", "py"}
    assert len(corpus.entries) == 8
    assert corpus.output == REPO_ROOT / "evals/fixtures/code-comments/seed/_corpus"


def test_snapshot_matches_lockfile() -> None:
    corpus = Corpus.load(MANIFEST)
    drifted, missing = check(corpus)
    assert drifted == []
    assert missing == []


def test_snapshot_rebuild_removes_stale_files(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    repo.mkdir()
    (repo / "source.go").write_text("package source\n")
    manifest = tmp_path / "manifest.yaml"
    manifest.write_text(
        "repo_root: .\n"
        "output: corpus\n"
        "lockfile: corpus.lock.json\n"
        "repos:\n"
        "  - id: sample\n"
        "    path: repo\n"
        "    language: go\n"
        "    files: [source.go]\n"
    )
    corpus = Corpus.load(manifest)
    snapshot(corpus, force=True)
    stale = corpus.output / "stale.go"
    stale.write_text("package stale\n")

    snapshot(corpus, force=True)

    assert not stale.exists()


def test_recall_ground_truth_is_fixed() -> None:
    expected = json.loads(
        (
            REPO_ROOT / "evals/fixtures/code-comments/seed/expected-findings.json"
        ).read_text()
    )
    assert expected["schema_version"] == 2
    assert "not exhaustive" in expected["scope"]
    assert len(expected["findings"]) == 4
    assert len({item["id"] for item in expected["findings"]}) == 4


def test_score_schema_separates_additional_and_context_dependent_findings() -> None:
    schema = yaml.safe_load(
        (
            REPO_ROOT / "evals/fixtures/code-comments/schemas/code-comments.yaml"
        ).read_text()
    )
    assert {
        "matched_expected_findings",
        "valid_additional_findings",
        "context_dependent_findings",
        "invalid_findings",
        "finding_classifications",
        "missed_expected_findings",
        "estimated_precision",
    } <= schema["fields"].keys()
    assert "false_positives" not in schema["fields"]
    assert schema["fields"]["finding_classifications"]["classes"] == [
        "matched_expected",
        "valid_additional",
        "context_dependent",
        "invalid",
        "non_comment",
    ]


def test_fixture_uses_same_model_for_generation_paths() -> None:
    fixture = yaml.safe_load(
        (REPO_ROOT / "evals/fixtures/code-comments/longe.yaml").read_text()
    )
    assert fixture["wt-coderev"]["model"] == "gemma4"
