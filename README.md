# WriteTighter

WriteTighter is a local, standalone Go CLI that checks technical prose, returns
machine-readable findings, and can optionally ask an LLM for narrowly scoped rewrite
suggestions. It is a neutral checking host — each check selects one versioned profile
that supplies the applicable dictionary and rule policy.

**Status:** Pre-release / private development.

## Quick start

```sh
# Build from source
go build -o writetighter ./cmd/writetighter

# Check stdin
echo "Create the file and then utilize it." | ./writetighter check --stdin --kind pr

# Check a directory of Markdown files
./writetighter check docs/ --kind description --format json

# List installed profiles
./writetighter profile list

# Explain a rule
./writetighter explain CORE.SENTENCE_LENGTH
```

## Repository layout

- `cmd/writetighter/` — CLI entry point
- `internal/app/` — command orchestration and exit policy
- `internal/config/` — XDG/project config and term-base validation
- `internal/document/` — Markdown/plain segmentation and source spans
- `internal/check/` — checker interface and implementations
- `internal/profile/` — bundle schemas, validation, resolution, embedding
- `internal/report/` — schema-v1 model and renderers
- `internal/llm/` — OpenAI-compatible client, prompt, response validation
- `internal/data/profiles/` — verified embedded default bundle
- `schemas/` — JSON Schemas for bundle and report contracts
- `testdata/` — fixtures for documents, expected output, profiles, term bases
- `docs/` — decisions, data provenance, configuration, agent workflow

## Documentation

- [Architecture decisions](docs/decisions/)
- [Data provenance and rights](docs/data-provenance.md)
- [Configuration guide](docs/configuration.md)
- [Agent workflow](docs/agent-workflow.md)
- [Profile authoring](docs/profile-authoring.md)

## License

Private development. Not cleared for public redistribution.

## Status

- Stage 0: ✅ Naming, rights, and contract records
- Stage 1: ⬜ Offline CLI skeleton and configuration
- Stage 2: ⬜ Document ingestion and report contract
- Stage 3: ⬜ Profile bundles, installation, and embedding
- Stage 4: ⬜ Deterministic rules and evaluation
- Stage 5: ⬜ Opt-in LLM advisor
- Stage 6: ⬜ Agent adoption
- Stage 7: ⬜ Release and maintenance
