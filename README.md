# vimpin

Pin Vim/Neovim plugin specs to explicit commit hashes, and let Renovate
drive the updates.

`vimpin` rewrites your existing plugin spec files to pin every plugin to an
explicit commit hash, inline, while keeping a human-readable annotation of
the original tag or branch. It pairs with [Renovate](https://docs.renovatebot.com/)
through a ready-made preset so commit bumps land as reviewable pull requests
instead of silent `:Lazy update` calls.

The approach extends the commit-pinning pattern that has become standard
for CI workflows to the Lua spec files that Neovim plugin managers consume.

> **Status:** alpha, used by author. The CLI surface is small (single `run`
> subcommand with mode flags) and unlikely to change incompatibly; the
> supported Lua spec shape may tighten as edge cases surface.

> **Scope:** vimpin aims to support the major Vim/Neovim plugin managers
> over time. **Currently only `lazy.nvim` Lua specs are supported.** packer.nvim,
> vim-plug, and lockfile-only flows are tracked in the roadmap below.

## Why

`lazy.nvim` happily honours `commit = "..."` in your spec, but most people
never reach for that field because there is no good update story without
external tooling. So plugins stay on a floating HEAD (`:Lazy update` moves
them) or on a `tag = "..."` that lazy.nvim resolves at install time but does
not lock — both of which leave the supply chain undefended.

`vimpin` makes the *commit* the source of truth, written directly into the
Lua spec, with the original tag/branch preserved as a comment for both
humans and Renovate to read.

## Quickstart

```bash
go install github.com/gr1m0h/vimpin/cmd/vimpin@latest

# Starting point: a normal lazy.nvim spec with a tag or branch hint
cat > lua/plugins/example.lua <<'LUA'
return {
  { "ggandor/leap.nvim", tag = "v0.1.5" },
  {
    "folke/which-key.nvim",
    branch = "main",
    keys = { "<leader>" },
  },
}
LUA

# Resolve every tag/branch to a commit and write it back inline
vimpin run

# Output: same file, now pinned and annotated
cat lua/plugins/example.lua
# return {
#   { "ggandor/leap.nvim", commit = "8a40d3aa...07b9079b" }, -- tag: v0.1.5
#   {
#     "folke/which-key.nvim",
#     commit = "3aab2147...0a44c15a", -- branch: main
#     keys = { "<leader>" },
#   },
# }

# Gate CI on every spec being pinned
vimpin verify
```

(Commit hashes elided with `...` in this README. The on-disk value is the
full 40-character hash that `git ls-remote` returns.)

## Canonical form

Every spec is rewritten into one of two shapes:

**Form A** — single-line spec, comment trails the closing brace:

```lua
{ "owner/repo", commit = "<40-hex>" }, -- tag: v0.1.5
```

**Form B** — multi-line spec, comment trails the commit field:

```lua
{
  "owner/repo",
  commit = "<40-hex>", -- branch: main
  keys = { "x" },
  config = function() end,
}
```

Two invariants hold across both forms:

1. **`commit` is the only authoritative ref.** `lazy.nvim` uses it; `tag`/
   `branch` Lua fields are removed by `vimpin run`.
2. **The `-- tag:` / `-- branch:` annotation lives on the same line as the
   commit value.** This is what both vimpin and Renovate read to know which
   upstream ref to track. The annotation must follow the commit value on a
   single line.

## Source of truth

The **commit SHA on disk is authoritative.** Once a spec is in canonical
form, vimpin will never change the SHA unless you explicitly ask via
`--update`. The annotation comment is a derived artefact: it records which
tag (or branch) the SHA was taken from. If the annotation drifts (someone
hand-edited it, or upstream rewrote a tag), `--verify` corrects the
annotation to match the SHA, never the other way around.

This is the foundation of vimpin's supply-chain story: the only path that
moves an SHA forward is one the operator typed themselves.

## Commands

vimpin exposes a single `run` subcommand whose mode is selected by flags.

```text
vimpin run [PATHS...]
  Default: pin field-form (tag=/branch=) specs to canonical commit form.
  Specs already in canonical form are a no-op. With no PATHS, scans the
  LazyVim default layout: lua/plugins/, lua/config/lazy.lua, init.lua,
  plugin/.

  --verify    SHA is source of truth. Reverse-resolve each commit hash on
              the remote, find the tag that points at it, and rewrite the
              annotation comment to match. The commit field is never
              touched. Use this to detect (and auto-correct) annotation
              drift after a tag rewrite or a hand-edit. Branch-annotated
              specs are left alone (a SHA can appear on many branches; no
              meaningful reverse lookup exists).

  --update    Bump each spec to the latest semver tag (or, for branch-
              annotated specs, the current branch HEAD). Both the commit
              field and the annotation are updated atomically. This is
              the ONLY mode that intentionally advances the commit SHA.

  --no-api    Offline structural check. Asserts every spec has a 40-hex
              commit field and a -- tag: / -- branch: annotation. No
              network calls. Useful as a fast CI pre-check before the
              network-bound --verify.

  --check     Do not write. Exit non-zero if any file would change. Can
              be combined with --verify or --update. Use this for CI.
```

`--verify`, `--update`, and `--no-api` are mutually exclusive.

### `-- vimpin:ignore`

Append `-- vimpin:ignore` to a spec to opt it out of every `vimpin run`
mode:

```lua
{ "internal/dev-plugin", dir = "~/code/plugin" }, -- vimpin:ignore
```

This is the supported escape hatch for local plugins, plugins you do not
want managed, or temporary experiments.

## Authentication and rate limits

vimpin shells out to `git ls-remote` to resolve tags and branches — it does
**not** call the GitHub REST API. Two consequences:

- **Authentication piggybacks on local git.** Private repos resolve through
  whichever credential helper your shell has configured (`git credential`,
  `gh auth setup-git`, SSH keys, etc.). vimpin sets `GIT_TERMINAL_PROMPT=0`
  so missing credentials fail fast rather than hanging on a password prompt.
- **The 60 req/h unauthenticated REST limit does not apply.** GitHub's git
  protocol endpoints are not subject to that quota, so even several hundred
  plugins resolve without a token.

Hosts other than `github.com` are not yet supported in the CLI: vimpin
constructs `https://github.com/<owner>/<repo>` from each spec's positional
string. Mirroring via local `git config` aliases works as a workaround;
first-class multi-host support is on the roadmap.

## Renovate integration

The companion preset
[`gr1m0h/vimpin-renovate-config`](https://github.com/gr1m0h/vimpin-renovate-config)
ships ready-to-use custom managers for the canonical form above. Add it to
your repo's `renovate.json`:

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["github>gr1m0h/vimpin-renovate-config"]
}
```

Renovate then opens a PR each time a newer tag is published, updating
both the commit hash and the annotation comment atomically. Because both
halves change in the same PR, drift between them is structurally
impossible while Renovate is the sole updater.

For branch-annotated specs, the preset's `git-refs` manager will also
open digest-bump PRs as the branch HEAD moves. This is opt-in: the
default preset enables it, but tag-only deployment is the recommended
posture if you want the strictest SHA-as-source-of-truth discipline.

See the preset's README for the layout constraints, recommended companion
config (`dependencyDashboard`, `prConcurrentLimit`, `schedule`), and known
limits.

## GitHub Actions

### Recommended: required check on every PR

Treat the read-only modes as **required status checks** on `main`. This
catches three failure modes in one place:

1. New specs that ship without a commit pin (`vimpin run` never ran).
2. Pin annotations that no longer match their commit (upstream tag rewriting
   or hand-edits).
3. Field-order violations that would silently break Renovate's regex.

```yaml
name: vimpin
on:
  pull_request:
    paths:
      - 'lua/**/*.lua'
      - 'init.lua'
permissions:
  contents: read
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: go install github.com/gr1m0h/vimpin/cmd/vimpin@latest
      - run: vimpin run --check                  # no rewrite required by this PR?
      - run: vimpin run --no-api                 # offline structural check
      - run: vimpin run --verify --check         # SHA <-> annotation aligned?
```

Configure `main` branch protection so both `verify / vimpin` jobs are required
before merge.

### Update workflow (optional)

A second workflow can run `vimpin run --update` via `workflow_dispatch` (or
schedule) to bump pinned specs to the latest semver tag, and open a PR with
the resulting changes. Pair with Renovate's `dependencyDashboard` so all
SHA-advancing operations remain reviewable.

### One-line usage with `vimpin-action`

The companion [`gr1m0h/vimpin-action`](https://github.com/gr1m0h/vimpin-action)
collapses the install-and-run boilerplate above into a single step:

```yaml
jobs:
  vimpin:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: gr1m0h/vimpin-action@v1
        with:
          mode: verify     # or: check, no-api, update
```

The action versions independently of the CLI, so its input surface can
evolve without forcing a vimpin release.

## Field-order constraint

For the bundled Renovate preset to recognise a spec, `commit` must be the
**first named field after the positional repo string**. vimpin emits this
layout by construction, so a regular `vimpin run` → commit cycle never
violates the constraint. Manual edits that move `commit` past other fields
(`event`, `keys`, `opts`, etc.) will be silently ignored by Renovate.

Compliant:

```lua
{ "owner/repo", commit = "...", event = "VeryLazy" }, -- tag: v1.0
{
  "owner/repo",
  commit = "...", -- tag: v1.0
  event = "VeryLazy",
}
```

Not compliant (Renovate will skip):

```lua
{ "owner/repo", event = "VeryLazy", commit = "..." }, -- tag: v1.0
```

The trade-off is intentional: a fixed field order is what makes a single
regex-based Renovate manager work reliably. Loosening the layout would
require a Lua parser inside Renovate itself, which is not something the
custom-manager interface supports today.

## Roadmap

Tracked as [GitHub Issues](https://github.com/gr1m0h/vimpin/issues?q=is%3Aissue+is%3Aopen+label%3Aenhancement). Highlights:

- **SARIF output for `verify`** ([#1](https://github.com/gr1m0h/vimpin/issues/1))
- **Diff-mode rewriting** ([#2](https://github.com/gr1m0h/vimpin/issues/2)) — incremental adoption path
- **Config file `.vimpin.yaml`** ([#3](https://github.com/gr1m0h/vimpin/issues/3)) — per-spec policy rules
- **`vimpin add <owner/repo>`** ([#4](https://github.com/gr1m0h/vimpin/issues/4))
- **packer.nvim adapter** ([#5](https://github.com/gr1m0h/vimpin/issues/5))
- **vim-plug adapter** ([#6](https://github.com/gr1m0h/vimpin/issues/6))
- **Multi-host clone URLs** ([#7](https://github.com/gr1m0h/vimpin/issues/7)) — gitlab.com, sr.ht, custom hosts
- **Semver `version = "..."` resolution** ([#8](https://github.com/gr1m0h/vimpin/issues/8))
- **Sigstore-style provenance** ([#9](https://github.com/gr1m0h/vimpin/issues/9))
- **`httptest`-backed git fixture** ([#10](https://github.com/gr1m0h/vimpin/issues/10)) — hermetic resolver tests

## Comparison

The peer choices for keeping `lazy.nvim` plugin versions reproducible:

| Approach                         | Locks?            | Source of truth       | Update flow                                |
|----------------------------------|-------------------|-----------------------|--------------------------------------------|
| `lazy-lock.json` (lazy.nvim)     | Records only      | Last `:Lazy sync`     | `:Lazy update` moves everything            |
| Hand-written `commit = "..."`    | Yes (commit pin)  | Lua spec              | Nothing automated; manual edits            |
| `vimpin` + Renovate preset       | Yes (commit pin)  | **The SHA itself**    | Renovate PRs; `--update` to bump locally   |

## Testing

`go test ./...` covers the scanner, the rewriter, and the canonical-form
golden outputs. The resolver layer is exercised end-to-end through
manual invocation against real GitHub remotes; an `httptest`-backed git
fixture is on the roadmap.

## Non-goals

- Replacing plugin managers — keep using `lazy.nvim`.
- Managing lazy-load configuration (`event`, `cmd`, `keys`) — vimpin only
  touches the pinning fields and the annotation comment.
- Cryptographic verification of commit contents — planned for a later
  phase via sigstore.

## License

MIT
