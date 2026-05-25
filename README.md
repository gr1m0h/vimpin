# vimpin

Pin Vim/Neovim plugin specs to explicit commit hashes, and let Renovate
drive the updates.

`vimpin` rewrites your existing plugin spec files to pin every plugin to an
explicit commit hash, inline, while keeping a human-readable annotation of
the original tag or branch. It pairs with [Renovate](https://docs.renovatebot.com/)
through a ready-made preset so commit bumps land as reviewable pull requests
instead of silent `:Lazy update` calls.

The approach mirrors the commit-pinning pattern that has become standard
for GitHub Actions workflows (e.g. [`pinact`](https://github.com/suzuki-shunsuke/pinact)),
applied to the Lua spec files that Neovim plugin managers consume.

> **Status:** alpha, used by author. The CLI surface is small (`run`,
> `verify`) and unlikely to change incompatibly; the supported Lua spec
> shape may tighten as edge cases surface.

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

## Commands

```text
vimpin run [PATHS...]
  Resolve tag/branch refs to commits and rewrite specs in canonical form.
  With no PATHS, scans the LazyVim default layout: lua/plugins/, lua/config/lazy.lua,
  init.lua, plugin/.

  --refresh    Re-resolve commit values for already-pinned specs against the
               current annotated ref (use after a quiet period to pick up
               commits Renovate has not yet PR'd).
  --check      Do not write; exit non-zero if any file would change (CI use).
  --dry-run    Do not write; print the planned new file contents to stdout.

vimpin verify [PATHS...]
  Check that every spec has a 40-hex commit value and a -- tag: / -- branch:
  annotation. Exit code is non-zero if any check fails.

  --strict     Additionally re-resolve each ref against the remote and report
               drift (commit no longer matches the annotated tag/branch).
```

### `-- vimpin:ignore`

Append `-- vimpin:ignore` to a spec to opt it out of `vimpin run` and
`vimpin verify`:

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

Renovate then opens a PR each time the annotated tag or branch moves,
updating both the commit hash and the annotation comment atomically. Because
both halves change in the same PR, drift between them is structurally
impossible while Renovate is the sole updater.

See the preset's README for the layout constraints, recommended companion
config (`dependencyDashboard`, `prConcurrentLimit`, `schedule`), and known
limits.

## GitHub Actions

Run `vimpin run --check` on every PR and let CI block merges that
introduce unpinned specs. Example workflow:

```yaml
name: vimpin
on:
  pull_request:
    paths:
      - 'lua/**/*.lua'
      - 'init.lua'
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: go install github.com/gr1m0h/vimpin/cmd/vimpin@latest
      - run: vimpin run --check
      - run: vimpin verify --strict
```

A second workflow can run `vimpin run` on a schedule (or via
`workflow_dispatch`) and open a PR with the resulting changes; pair with
Renovate's `dependencyDashboard` so all pin movements stay reviewable.

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

- **`vimpin add <owner/repo>`** — interactive spec creation that fetches
  the latest release tag (or default branch) and writes a canonical entry.
- **packer.nvim adapter** — apply the same pattern to packer specs.
- **vim-plug adapter** — apply the same pattern to vim-plug.
- **Multi-host clone URLs** — first-class `gitlab.com`, `git.sr.ht`, and
  custom-host support so non-github specs do not require a git URL rewrite.
- **Semver `version = "..."` resolution** — parse `version = "^0.1"` style
  ranges, pick the highest matching tag on the remote, and pin to its
  commit.
- **Sigstore-style provenance** — verify commit signatures during `verify
  --strict` (today vimpin trusts the remote response from `git ls-remote`).

## Comparison

| Tool / artifact                  | Locks?            | Source of truth       | Update flow                                |
|----------------------------------|-------------------|-----------------------|--------------------------------------------|
| `lazy-lock.json` (lazy.nvim)     | Records only      | Last `:Lazy sync`     | `:Lazy update` moves everything            |
| Hand-written `commit = "..."`    | Yes (commit pin)  | Lua spec              | Nothing automated; manual edits            |
| `pinact` (GitHub Actions)        | Yes (commit pin)  | Workflow `.yml`       | Renovate PRs against the pinned hash       |
| `vimpin` + Renovate preset       | Yes (commit pin)  | Lua spec              | Renovate PRs against the pinned hash       |

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
