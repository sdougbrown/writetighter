# Codex plugin

Manually pasting the `writetighter` commands into Codex works, but it leaves the agent to work out the workflow on its own. The WriteTighter plugin packages a `tighten` skill that knows the two-command model, the verification rules, and how to handle an unconfigured `revise` without blocking.

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

  Without a configured endpoint, the skill falls back to self-revision — see below.

- Install a Codex CLI version that supports plugins.

Confirm the binary is reachable before installing the plugin:

```bash
writetighter --help
```

## Setup

1. Register WriteTighter's marketplace from GitHub:

   ```bash
   codex plugin marketplace add sdougbrown/writetighter
   ```

2. Install WriteTighter from the registered marketplace:

   ```bash
   codex plugin add writetighter@writetighter
   ```

3. Start a new Codex task. Plugin skills load when a task starts, so an
   already-open task will not pick up the integration.

For GitHub and repository-root installs, Codex and Claude Code both consume
`.claude-plugin/marketplace.json`. That manifest points to
`marketplace/plugins/writetighter`, where Codex loads
`.codex-plugin/plugin.json`.

### Installing from a local checkout or branch

The GitHub source above is the normal installation path. To test local plugin
changes, register the repository root instead:

```bash
codex plugin marketplace add /absolute/path/to/writetighter
```

To test a Git branch without cloning it, pin that ref when registering the
GitHub source:

```bash
codex plugin marketplace add sdougbrown/writetighter --ref feature/my-plugin-change
```

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

If you skip model setup, `revise` exits with code 3. Codex does not expose a
nested subagent with a separate model, so the skill does not block — it falls
back to **self-revision**: it runs `writetighter prompt --kind <kind> --format json` to get the revision rubric, then Codex produces the `revisions[]` JSON
shape itself and hands it to the same verification step. If you would rather use
a dedicated model, run `writetighter config` and use `revise` directly. Full
details ship inside the plugin at `skills/tighten/references/host-codex.md`.

## Verification

```bash
codex plugin list
```

In a new Codex task, asking Codex to "tighten this" or "run writetighter on this
doc" should trigger the skill.

## Troubleshooting

### Codex cannot find the binary

The skill runs `writetighter` by command name. Make sure the directory
containing the binary is on the `PATH` inherited by Codex, then start a new task.

### Codex cannot find the marketplace

Check whether the marketplace was registered:

```bash
codex plugin marketplace list
```

If `writetighter` is missing, retry with the exact owner and repository name:

```bash
codex plugin marketplace add sdougbrown/writetighter
```

This step needs network access to GitHub. If GitHub registration still fails
and you have a local checkout, register its absolute repository-root path
instead.

### The skill is installed but does not trigger

Start a new Codex task after installation. Skills are not added retroactively to
an existing task.

## See also

- [Claude Code plugin](/claude-plugin) — the equivalent integration for Claude
  Code.
- [Agent integration](/agent-integration) — the raw command workflow the skill
  formalizes.
- [CLI reference](/cli) — every `writetighter` command and flag.