# WriteTighter feature verification map

Each file below is a user-facing feature with driving steps for the CLI
harness. A proof that drives only one entry point is incomplete when the map
lists others.

| Feature | File | Needs model? |
|---|---|---|
| Deterministic lint | `lint.md` | no |
| Contextual revision (`revise`/`rewrite`) | `revise.md` | stub |
| Revision guidance export (`prompt`) | `prompt.md` | no |
| Model configuration (`config`) | `config.md` | stub (preflight) |
| Profile management (`profile`) | `profile.md` | no |

Secondary commands not mapped (low user risk): `help`, `version` (covered by
the Doctor check in `../SKILL.md`), `explain` (single-rule help text; drive
as `writetighter explain CORE.SENTENCE_LENGTH`, exit 0; a bogus rule ID exits
2 with `rule not found:`).
