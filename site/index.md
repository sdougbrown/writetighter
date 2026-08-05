---
layout: home

hero:
  name: WriteTighter
  tagline: Your <i>automatic</i> red pen. Fight back against <i>Combat Slop</i>
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
    title: Two tools for two failures
    details: <code><b class="wt-bold">lint</b></code> for structural density and paragraph length. <code><b class="wt-bold">revise</b></code> for semantic slop like hedging and broken logic.
  - icon: ✂️
    title: Never edits your file
    details: Both commands report findings and suggestions. A human or the calling agent decides what changes. WriteTighter has no write path to your source.
  - icon: 🪶
    title: Tiny skill file, simple CLI
    details: The profile and dictionary are embedded in the binary. Wire it into an agent with a few lines. Use a local model or hand a cheap subagent the revision prompt.
  - icon: 🧽
    title: Combat Slop defense
    details: Stop reading hyper-dense, unreadable technical word salad. Clean up the noise before it reaches your eyes.
---

## One real example

<p><del class="wt-cut">// Cache resolution — getEntry(cacheKey) is null on a miss; an unguarded read is an NPE.</del></p>
<span class="wt-note">revise, kind=code-comment — clarification: "Which specific object or field is being accessed during the 'unguarded read' that triggers the NPE?"</span>

<p><ins class="wt-keep">// Cache resolution — getEntry(cacheKey) returns null on a miss. Reading the returned entry without a guard throws a NullPointerException.</ins></p>

*Adapted from a real PR review — identifiers changed, the clarification is unedited `revise` output.*

## Install

```bash
curl -fsSL https://writetighter.douggo.com/install.sh | bash
```

Or with Go: `go install github.com/sdougbrown/writetighter/cmd/writetighter@latest`

Binaries for all platforms on [GitHub Releases](https://github.com/sdougbrown/writetighter/releases/latest).

## Try it in <del class="wt-cut">one</del> <ins class="wt-keep">two</ins> line<ins class="wt-keep">s</ins>

```bash
echo "The unprompted inclusion of the StoreProvider jest auto-mock was necessary because the context-reading hook required a real StoreContext export to ensure referential stability under tests, specifically to prevent the no-provider fallback from triggering during the array-map execution." |
  writetighter lint --stdin --kind description
```

```bash
echo "The unprompted inclusion of the StoreProvider jest auto-mock was necessary because the context-reading hook required a real StoreContext export to ensure referential stability under tests, specifically to prevent the no-provider fallback from triggering during the array-map execution." |
  writetighter revise --stdin --kind description
```


More examples on the [before/after page](/before-after) if you want to see them. Your models may vary. 😀
