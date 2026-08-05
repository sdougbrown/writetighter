# Claude Code plugin

Manually pasting the `writetighter` commands into a skill works, but it leaves the agent to work out the workflow on its own. The WriteTighter plugin packages a `tighten` skill that knows the two-command model, the verification rules, and how to fall back to subagent delegation when `revise` is unconfigured.

This is not a separate implementation. The skill invokes the same `writetighter lint` / `revise` / `prompt` CLI documented in the [CLI reference](/cli). WriteTighter never edits files — the skill instructs your agent to apply the findings it can verify.

## Prerequisites

- Install WriteTighter and make sure `writetighter` is on your `PATH`.

  ```bash
  curl -fsSL https://writetighter.douggo.com/install.sh | bash
  ```

- For native contextual revision, run the setup wizard once so `revise` has an endpoint:

  ```bash
  writetighter config
  ```

  Without a configured endpoint, the skill falls back to subagent delegation — see below.

- Install a Claude Code CLI version that supports plugins.

Confirm the binary is reachable before installing the plugin:

```bash
writetighter --help
```

## Setup

1. Register the marketplace from GitHub:

   ```bash
   claude plugin marketplace add sdougbrown/writetighter
   ```

   No local checkout is required; Claude Code fetches the manifest and plugin
   files from the repository's default branch.

2. Install WriteTighter from the registered marketplace:

   ```bash
   claude plugin install writetighter@writetighter
   ```

3. Start a new Claude Code session. Plugin skills load when a session starts, so
   an already-open session will not pick up the integration.

### Installing from a local checkout or branch

If you are working against a fork or an unmerged branch, register the
marketplace from an absolute path instead:

```bash
claude plugin marketplace add /absolute/path/to/writetighter
```

Or pin a branch on GitHub directly, without cloning:

```bash
claude plugin marketplace add sdougbrown/writetighter@branch-name
```

Pass the repository root, not the `marketplace/` subdirectory —
`.claude-plugin/marketplace.json` lives at the root and the plugin `source`
resolves relative to that.

## What the skill does

The bundled `tighten` skill runs your drafted prose through both commands before
you ship:

- `writetighter lint --kind <kind> --format json` — deterministic rule findings.
- `writetighter revise --kind <kind>` — contextual rewrites and clarification
  questions.

It then applies only the changes it can verify (a `replacement` must preserve
every command, path, identifier, and number), re-lints, and surfaces any
clarification question it cannot answer instead of guessing.

## When `revise` is not configured

If you skip model setup, `revise` exits with code 3. The skill does not block —
it falls back to **subagent delegation**: it runs `writetighter prompt --kind <kind> --format json` to get the revision rubric, then spawns a Claude Code
subagent (via the Agent tool, typically a cheaper model) that returns the same
`revisions[]` JSON shape. The verification rules still apply to the subagent's
output. Full details ship inside the plugin at
`skills/tighten/references/host-claude.md`.

## Verification

```bash
claude plugin list
claude plugin details writetighter@writetighter
```

In a new session, asking Claude to "tighten this" or "run writetighter on this
PR description" should trigger the skill.

## Troubleshooting

### Claude Code cannot find the binary

The skill runs `writetighter` by command name. Make sure the directory containing
the binary is on the `PATH` inherited by Claude Code, then start a new session.

### Claude Code cannot find the marketplace

For a local checkout, pass the absolute path to the repository root, not the
`marketplace/` subdirectory. For the GitHub form, confirm the manifest has
landed on the branch you are targeting; `owner/repo` resolves against the default
branch only.

### The skill is installed but does not trigger

Start a new Claude Code session after installation. Skills are not added
retroactively to an existing session.

## See also

- [Codex plugin](/codex-plugin) — the equivalent integration for OpenAI Codex.
- [Agent integration](/agent-integration) — the raw command workflow the skill
  formalizes.
- [CLI reference](/cli) — every `writetighter` command and flag.