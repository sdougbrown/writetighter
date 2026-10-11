# Profile management (profile)

## Sub-features

- `profile list`: installed profiles including the embedded default
  (`software-docs-en@0.6.0`), with sha256.
- `profile verify <ID@VERSION|bundle-path>`: validates a profile or bundle.
- `profile install <bundle>`: installs a profile bundle into the user config
  area (scratch `XDG_CONFIG_HOME` in verification).

## How to get to it (user POV)

The user lists what is installed, verifies a pinned `ID@VERSION`, and
installs a new profile bundle from a local path.

## Driving it with CLI

```sh
"$WT/writetighter" profile list
echo exit=$?   # expect 0; line contains "embedded" and "software-docs-en@0.6.0"
"$WT/writetighter" profile verify software-docs-en@0.6.0
echo exit=$?   # expect 0
"$WT/writetighter" profile verify software-docs-en@9.9.9; echo exit=$?   # expect 2 (not installed)
# Install path: build a minimal bundle or use testdata/ if a bundle is present.
# After install, `profile list` must show the new profile ID@VERSION and exit 0.
```

Observable end state: `list` shows the embedded profile with a stable sha256;
`verify` of an unknown version exits 2 with a clear message; a successful
`install` makes the new profile appear in `list`.

## Gotchas

- The embedded profile sha256 is content-addressed: if the profile source
  changed, the sha256 in `profile list` output changes — update map/proof
  expectations in the same change.
- `profile install` writes into the config area, so always run with the
  scratch `XDG_CONFIG_HOME` (see `../SKILL.md` isolation rules).
- Lint results are pinned to the profile; a verification recipe that asserts
  finding wording must pin `ID@VERSION` semantics of the embedded profile.
