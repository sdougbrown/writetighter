# 0002: Profile Model

**Date:** 2026-07-19
**Status:** Accepted

## Context

WriteTighter needs a core abstraction that separates vocabulary data from rule policy. Different vocabularies often require different thresholds, severities, and rule identities; a vocabulary-only swap is not enough. At the same time, combining multiple profiles in one check would add significant complexity with unclear first-year value.

## Decision

Use a one-active-profile model with three layers:

### Dictionary

Lexical data only. An entry identifies a term, its allowed parts of speech, its status, explicit alternatives, and evidence references. The checker must not infer approval, disapproval, part of speech, or alternatives from raw frequency.

Valid entry statuses:
- `preferred` — explicitly recommended by the profile.
- `allowed` — accepted but not preferred.
- `discouraged` — explicitly reviewed and paired with a reason.
- `observed` — found in source material but not reviewed as policy.

`parts_of_speech` may be empty only for an `observed` entry. `preferred`, `allowed`, and `discouraged` entries require at least one reviewed POS value.

### Profile

Combines exactly one dictionary with rule enablement, thresholds, severities, enforcement states, language, supported document kinds, provenance, license, and claims. Has a stable ID, semantic version, and content hash. Multiple versions can be installed; exactly one is resolved before input text is read.

### Project term base

A project-local overlay in `.writetighter.toml`. Not silently merged into the published profile.

Lookup precedence:
1. exact project term with `override = true` and a non-empty `reason`.
2. exact profile dictionary entry.
3. non-conflicting project addition.
4. active profile's unknown-term policy.

A project entry conflicting with a `discouraged` profile entry must set `override = true` and provide a reason. Otherwise configuration fails with exit code 2.

## Consequences

- Profile authors maintain separate dictionary and rule files.
- Users pin explicit profile versions; "latest" resolution is not supported.
- The same normalized bundle format works embedded and installed.
