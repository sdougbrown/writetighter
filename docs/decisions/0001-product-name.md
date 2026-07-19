# 0001: Product Name

**Date:** 2026-07-19
**Status:** Accepted

## Context

The checker needed a stable product name that:
- Identifies the checking function without claiming certification.
- Does not conflict with existing writing-checker CLIs.
- Does not imply one writing profile (STE) is the identity of the host.
- Works as a repository name, module suffix, binary name, and filesystem/config prefix.

The working name `stecheck` was rejected because it incorrectly makes one potential ASD-STE100 profile sound like the identity of the host. The name `noslop` was considered and rejected because a Rust CLI already uses that name in the agent-quality-gate space, and "slop" does not identify a language or domain. The name `write-tight` was considered and rejected because an existing writing-checker CLI already occupies that name.

## Decision

Use **WriteTighter** as the product name and `writetighter` as the checker repository, module suffix, binary name, and filesystem/config prefix.

## Name search results

Exact searches on 2026-07-19 (verified by plan author before execution):

- GitHub: no `writetighter` repository or package found.
- npm: no `writetighter` package found.
- PyPI: no `writetighter` package found.
- crates.io: no `writetighter` crate found.

Repeat the search immediately before public release; if a conflicting technical-writing tool has appeared, stop and amend this plan instead of improvising another name.

## Consequences

- Repository names are kept out of serialized profile identity. The first profile has the stable machine ID `software-docs-en`, regardless of where its source repository lives.
- The name describes the intended outcome, not an automatic rewrite or certification claim. Human-facing text says that WriteTighter checks writing and offers advice; it does not imply that the tool silently edits text or proves correctness.
