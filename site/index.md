---
layout: home

hero:
  name: WriteTighter
  tagline: A red pen for prose that got a little too fond of itself — deterministic lint plus contextual rewrites, never auto-applied.
  actions:
    - theme: brand
      text: See it work
      link: /before-after
    - theme: alt
      text: CLI Reference
      link: /cli
    - theme: alt
      text: GitHub
      link: https://github.com/sdougbrown/writetighter

features:
  - icon: ✒️
    title: Two commands, different catches
    details: lint is deterministic — sentence length, dense paragraphs, no model required. revise is contextual — hedging, pronouns pointing at nothing, missing relationships, caught by a model you configure.
  - icon: ✂️
    title: Never edits your file
    details: Both commands report findings and suggestions. A human or the calling agent decides what changes. WriteTighter has no write path to your source.
  - icon: 🪶
    title: No skill file, just a CLI
    details: The profile and dictionary are embedded in the binary. Wire it into an agent with a few lines — locally, or by handing a cheap subagent the revision prompt directly.
  - icon: 📜
    title: Built for agents reading docs
    details: Every extra hedge and throat-clearing clause a model writes is context another model has to pay for later. Tight prose is a cost saving, not just a style preference.
---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/sdougbrown/writetighter/main/install.sh | bash
```

Or with Go: `go install github.com/sdougbrown/writetighter/cmd/writetighter@latest`

Binaries for all platforms on [GitHub Releases](https://github.com/sdougbrown/writetighter/releases/latest).

## Try it in one line

```bash
echo "It's worth noting that this configuration change, while not strictly required, may potentially improve performance in certain scenarios." |
  writetighter lint --stdin --kind description
```

That sentence has a home on the [before/after page](/before-after) — with the cut marked in red and the replacement in green.
