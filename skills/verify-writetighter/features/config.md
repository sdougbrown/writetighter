# Model configuration (config)

## Sub-features

- `config` (no args): prints the current model configuration; when no usable
  config exists it starts the interactive wizard (in a terminal). With a
  missing scratch config it can fall back to and print the real
  `~/.config/writetighter/config.toml` — always seed the scratch config
  first (see `../SKILL.md` Launch).
- `config --wizard`: interactive setup — URL/port, auth choice, model
  selection, structured-output preflight, atomic write with 0600 permissions.
- Auth choices: no key, key in config (0600), key via environment variable.
  Keys are never accepted as command-line arguments.
- Preflight picks `json_schema` → `json_object` → `prompt_json` by what the
  endpoint/model supports.
- Non-interactive `revise` with missing config fails with a `writetighter
  config` hint (exit 2); interactive `revise` auto-starts the wizard.

## How to get to it (user POV)

The user runs `writetighter config --wizard` in a terminal, answers the
prompts, and ends with a working `~/.config/writetighter/config.toml`
(mode 600) they never hand-edit.

## Driving it with PTY harness

The stub must be running (see `../SKILL.md` Launch). Prompt order is:
API URL/port first, then authentication, then model selection — despite the
banner mentioning keys first:

```python
import os, pty, time
pid, fd = pty.fork()
if pid == 0:
    os.environ["XDG_CONFIG_HOME"] = os.environ["WT_CFG"]   # scratch dir
    os.execv(os.environ["WT_BIN"], ["writetighter", "config", "--wizard"])
def rd(t=0.8):
    out = b""; time.sleep(t)
    try: out = os.read(fd, 65536)
    except OSError: pass
    return out.decode(errors="replace")
assert "OpenAI-compatible API URL" in rd()          # banner + URL prompt
os.write(fd, b"127.0.0.1:8731\n"); assert "Authentication" in rd()
os.write(fd, b"1\n")                                # 1 = No API key
assert "Select a model" in rd(1.2)                  # stub-model listed
os.write(fd, b"\n")                                 # accept first model
out = rd(1.5)
assert "Wrote" in out and "with mode json_schema" in out
```

Then verify the written state:

```sh
stat -c %a "$XDG_CONFIG_HOME/writetighter/config.toml"   # expect 600
grep -q 'model = "stub-model"' "$XDG_CONFIG_HOME/writetighter/config.toml"
"$WT/writetighter" config | grep -q 'base_url = "http://127.0.0.1:8731/v1"'
echo exit=$?   # expect 0
```

Observable end state: wizard completes with a preflight success line, the
scratch config file exists with mode 600 and the stub URL, and plain
`config` prints that configuration back.

## Gotchas

- Input order is NOT display order: answer the URL/port prompt first, then
  the auth menu. Answering the auth value first makes the URL prompt consume
  it (a bare integer is a valid port!) and the wizard fails with
  `authentication selection must be 1, 2, or 3`.
- Never run the wizard with the user's real `XDG_CONFIG_HOME` — it writes
  `config.toml` for real.
- `response_mode` in the written config depends on stub preflight: with the
  shipped stub it is `json_schema` (the stub accepts `response_format`).
  Assert `json_schema` only while the stub keeps that behavior.
- Keys must never appear in argv; proofs exercise "No API key" (option 1)
  unless testing key handling deliberately.
