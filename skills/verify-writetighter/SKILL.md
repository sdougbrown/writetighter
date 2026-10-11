---
name: verify-writetighter
description: Verify the writetighter CLI (Go, Markdown revision harness) by building it, driving lint/revise/prompt/config/profile like a user, and capturing evidence. Run before claiming user-facing work is done, or whenever behavior at the CLI surface needs proof.
---

# Verify: writetighter

Drive the real `writetighter` binary at the CLI surface and capture evidence.
Read `features/README.md` for the app overview and `features/` for per-feature
driving steps.

## Isolation rules (read first)

- This is a single-user CLI. Every invocation reads config from
  `$XDG_CONFIG_HOME/writetighter/config.toml` (default `~/.config/writetighter/`).
  **Always export `XDG_CONFIG_HOME` to a per-run scratch dir AND create
  `$XDG_CONFIG_HOME/writetighter/config.toml` (even an empty file) before the
  first invocation** — a missing scratch config makes writetighter fall back to
  the real `~/.config/writetighter/config.toml`.
- `profile install` writes profile bundles under
  `$XDG_DATA_HOME/writetighter/profiles`; set a scratch `XDG_DATA_HOME`
  when driving profile installs.
- `lint`, `prompt`, `explain`, `profile`, `version` never touch the network.
- `revise`/`rewrite` call an OpenAI-compatible endpoint. For offline proof,
  run `helpers/llm_stub.py` on a scratch localhost port and point the scratch
  config at it. Never point a verification run at the user's real configured
  model.
- It is safe to run two verification sessions side by side: give each its own
  scratch dir, scratch port, and build output path.

## Launch

```sh
WT=$(mktemp -d /tmp/wt-verify.XXXXXX)
go build -o "$WT/writetighter" ./cmd/writetighter   # run once per session
```

Ready check: `"$WT/writetighter" version` prints `writetighter <version>` plus
an `embedded profile: software-docs-en@<version>` line, exit 0.

There is no daemon. For feature proofs that need the model stub:

```sh
python3 skills/verify-writetighter/helpers/llm_stub.py 8731 &   STUB_PID=$!
kill -0 "$STUB_PID" 2>/dev/null || { echo "stub failed to start (port taken?)"; exit 1; }
export XDG_CONFIG_HOME="$WT/cfg"
export WT_BIN="$WT/writetighter" WT_CFG="$XDG_CONFIG_HOME"
mkdir -p "$XDG_CONFIG_HOME/writetighter"
sed 's/PORT_STUB/8731/' <<'TOML' > "$XDG_CONFIG_HOME/writetighter/config.toml"
[llm]
provider = "openai-compatible"
base_url = "http://127.0.0.1:PORT_STUB/v1"
model = "stub-model"
response_mode = "prompt_json"
TOML
touch "$XDG_CONFIG_HOME/writetighter/config.toml"   # non-stub flows: empty file suppresses the real-config fallback
```

## Doctor

Run this whenever anything looks off; it answers "is this instance worth
driving?":

```sh
"$WT/writetighter" version && "$WT/writetighter" profile list
```

Pass criteria: `version` prints the tool version and embedded profile line;
`profile list` shows the embedded `software-docs-en` row; both exit 0. If
either fails, the build is broken — rebuild before verifying behavior.

## Drive

Deterministic commands are direct CLI calls with output and exit-code
assertions (recipes in `features/lint.md`, `features/prompt.md`,
`features/profile.md`):

```sh
"$WT/writetighter" lint --text "Restart the service after changing the file." \
  --kind pr --fail-on warning
echo exit=$?   # expect 0: the 9-word sentence is within limits; the fixture below trips a real finding
```

For a real finding, use a fixture with an over-long sentence:

```sh
printf 'This single overly long demonstration sentence deliberately packs far more words than the description profile permits because it keeps going well past the twenty-five word threshold with clause after clause appended.\n' > "$WT/fixture.md"
"$WT/writetighter" lint "$WT/fixture.md" --kind description --format json --fail-on warning
echo exit=$?   # expect 1, JSON contains CORE.SENTENCE_LENGTH
```

`revise` (offline stub, from `features/revise.md`):

```sh
printf 'Restart the service after changing the file.\n' > "$WT/revise-fixture.md"
"$WT/writetighter" revise "$WT/revise-fixture.md" --kind procedure --format json
```

Expect exit 0 and a `revisions` array with one `rewrite` whose `source_text`
matches the fixture sentence — and the fixture file unchanged.

`config --wizard` needs a PTY (the stub must be running). Prompt order is:
API URL/port first, then authentication choice, then model selection:

```python
import os, pty, time
pid, fd = pty.fork()
if pid == 0:
    os.environ["XDG_CONFIG_HOME"] = os.environ["WT_CFG"]  # scratch dir
    os.execv(os.environ["WT_BIN"], ["writetighter", "config", "--wizard"])
time.sleep(0.8); os.read(fd, 65536)          # drain banner
os.write(fd, b"127.0.0.1:8731\n")            # URL/port prompt
time.sleep(0.6); os.read(fd, 65536)          # auth menu
os.write(fd, b"1\n")                         # 1 = No API key
time.sleep(1.2); os.read(fd, 65536)          # model list (stub-model)
os.write(fd, b"\n")                          # accept first model
time.sleep(1.5); out = os.read(fd, 65536)    # "Wrote .../config.toml with mode json_schema"
```

Prefer stable handles: match prompt substrings
(`OpenAI-compatible API URL`, `Authentication [1]:`, `Select a model`),
JSON field names, and exit codes — never screen positions.

## Evidence

Capture to `skills/verify-writetighter/evidence/<run-id>/` (gitignored):

- full command transcripts with exit codes;
- JSON response bodies for `lint --format json` and `revise --format json`;
- for `revise`: a checksum of the fixture before and after (`revise` must
  never modify its target);
- for `config`: the PTY transcript and the written config file (`stat -c %a`
  must print `600`). Redact any `api_key` line before copying `config.toml`
  into evidence, and `chmod 600` copied evidence files.

Proof standard: the action and the resulting state, not just the final
screen. A `revise` proof shows the suggestion AND the untouched fixture; a
`lint --fail-on` proof shows the exit code, not only the findings array.
Exercise the real user path — never test-only hooks (there are none).

## Cleanup

- Kill only what you started: `kill "$STUB_PID"` for the stub (record `$!`
  when launching). Never `pkill` by name.
- `rm -rf "$WT"` removes the scratch build, config, and fixtures.
- Evidence under `skills/verify-writetighter/evidence/<run-id>/` survives
  cleanup — cleanup must not touch it. If a verification iteration failed,
  still run cleanup before retrying so no stub or scratch dir is stranded.

## Maintaining this skill

- The `features/` map is the repo's maintained verification source. When a
  change alters a feature's driving steps or proof state, update that
  feature's map file in the same change. When a new user-facing command or
  flag surface ships, add its map file.
- If the stub's canned finding stops matching (profile wording, principle
  IDs, response schema in `internal/llm/revise.go`), update
  `helpers/llm_stub.py` in the same change.
- Exit-code contract (0/1/2/3) is documented in `cmd/writetighter/main.go`
  and the help output; keep recipes aligned with it.
