# vimpin

A universal version pinner for Vim/Neovim plugins.

`vimpin` is to Vim/Neovim plugins what [pinact](https://github.com/suzuki-shunsuke/pinact) is to GitHub Actions: it pins every plugin to an explicit commit hash via a TOML manifest, and integrates with [Renovate](https://docs.renovatebot.com/) so updates flow through reviewable pull requests instead of silent `:Lazy update` calls.

> **Status:** alpha, used by author. Schema may change.

## Why

`lazy.nvim`'s lockfile only *records* the current commit — it does not *lock* it. `:Lazy update` will happily move every plugin to a new commit the moment you forget to pin specs explicitly. For supply-chain-conscious setups this is unacceptable.

`vimpin` makes the pin the source of truth and reduces the lockfile to a derived artifact.

### How vimpin compares

| Tool / artifact                  | Locks?            | Source of truth       | Update flow                                |
|----------------------------------|-------------------|-----------------------|--------------------------------------------|
| `lazy-lock.json` (lazy.nvim)     | Records only      | Last `:Lazy sync`     | `:Lazy update` moves everything            |
| `pinact` (GitHub Actions)        | Yes (commit pin)  | Workflow `.yml`       | Renovate PRs against the pinned hash       |
| `vimpin` alone                   | Yes (commit pin)  | `vimpin.toml`         | `vimpin pin --refresh` or manual edit      |
| `vimpin` + Renovate preset       | Yes (commit pin)  | `vimpin.toml`         | Renovate PRs against the pinned hash       |

## Quickstart

```bash
# Install (Go 1.24+)
go install github.com/gr1m0h/vimpin/cmd/vimpin@latest

# Write a manifest
cat > vimpin.toml <<'TOML'
schema = "https://vimpin.io/schema/v1"

[settings]
default_host = "github.com"
allow_hosts = ["github.com"]

[[plugin]]
repo = "ggandor/leap.nvim"
tag = "v0.1.5"
layer = "user"
group = "core"
TOML

# Resolve the tag to a commit hash
vimpin pin

# Verify everything is locked
vimpin verify --strict

# Generate the lazy.nvim spec
vimpin generate --adapter lazy --output lua/plugins/_generated.lua
```

## Manifest schema

| Field      | Required | Description |
|------------|----------|-------------|
| `repo`     | yes      | `owner/name` on the host |
| `commit`   | after pin | 40-char hex commit hash — the actual lock |
| `tag`      | optional | Renovate update hint (tag tracking) |
| `branch`   | optional | Renovate update hint (branch HEAD tracking) |
| `host`     | optional | overrides `settings.default_host` |
| `layer`    | optional | `user` (default) or `override` for distribution patches |
| `group`    | optional | filter target for `vimpin generate --groups` |
| `reason`   | optional | human comment, surfaced in adapter output |

The `schema` field is currently validated by exact string match against
`https://vimpin.io/schema/v1`. The URL is a stable identifier, not a live
endpoint — vimpin does not fetch it. Schema bumps (`/v2`, …) will be
gated by an explicit migration path before release.

### Settings

```toml
[settings]
default_host = "github.com"        # host used when [[plugin]] omits "host"
allow_hosts  = ["github.com"]      # whitelist; unknown hosts are rejected
```

- `allow_hosts` is a **strict whitelist**. Any plugin whose effective host
  is not present in the list fails validation (`pin`, `verify`, `generate`
  all reject the manifest). If `allow_hosts` is omitted, `github.com` is
  implicitly allowed and any other host is rejected.
- There is no `deny_hosts` field; if you need exclusion semantics, invert
  the question and shorten `allow_hosts`.

### Resolution precedence

`commit > tag > branch` across three axes:

1. **Install**: whatever ref is highest-priority among those present is what gets checked out.
2. **Renovate**: when both are present, tag tracking is preferred over branch HEAD tracking.
3. **Add** (roadmap, see below): `vimpin add owner/repo` will default to tag if a release exists, else default branch.

## Commands

```text
vimpin pin                  Resolve tag/branch → commit, write back
  --refresh                 Re-resolve entries that already have a commit
                            (tag entries get re-pointed at the latest commit
                             the tag currently resolves to; branch entries at
                             the branch's current HEAD)
vimpin verify               Confirm every entry is pinned to a 40-hex commit
  --strict                  Also re-check tag/branch ↔ commit alignment by
                            re-resolving each ref on the remote
vimpin generate             Emit plugin-manager-specific spec
  --adapter lazy            (default) lazy.nvim spec
  --groups core,work        Filter by group
  --output PATH             Write to file (default stdout)
```

### Roadmap (not yet implemented)

- `vimpin add <owner/repo>` — interactive entry creation
- `vimpin adopt / disown / status` — selective source-of-truth helpers for
  LazyVim users (see [LazyVim integration](#lazyvim-integration))

## Authentication and rate limits

vimpin shells out to `git ls-remote` to resolve tags and branches; it does
**not** call the GitHub REST API. This has two practical implications:

- **Authentication piggybacks on local git.** Private repos resolve through
  whatever credential helper your shell has configured (`git credential`,
  `gh auth setup-git`, SSH keys, etc.). vimpin sets
  `GIT_TERMINAL_PROMPT=0` so missing credentials fail fast rather than
  hanging on a password prompt.
- **The 60 req/h unauthenticated REST limit does not apply.** GitHub's
  git protocol endpoints are not subject to the REST rate limit, so even
  several hundred plugins resolve without a token. If you do hit a wall
  it will be from `git` itself (timeouts, host-level abuse limits), not
  the REST quota.

If you mirror plugins on another host (`gitlab.com`, `git.sr.ht`, internal
gitea, etc.) add it to `settings.allow_hosts` and ensure the relevant
git remote credentials are reachable from where you run `vimpin`.

## LazyVim integration

LazyVim does not pin its bundled plugins by default
([`version = false`](https://www.lazyvim.org/configuration/lazy.nvim) is
the recommended setting). `lazy-lock.json` records *which* commits are
currently installed, but it does not *enforce* them — `:Lazy update` will
move them on the next sync.

For supply-chain-conscious users on LazyVim:

- **distribution layer**: pin LazyVim itself with `version = "^14"` (major
  lock) or a commit hash.
- **vimpin layer**: opt in plugins individually with `layer = "override"`
  for LazyVim-managed plugins, or `layer = "user"` for plugins you brought.

`layer = "override"` is **the primary mechanism vimpin offers to LazyVim
users**, not a last-resort escape hatch. It is the normal way to make a
LazyVim-bundled plugin reproducible.

### Selective Source of Truth (recommended model)

The realistic operating model for a LazyVim user is not "pin every plugin"
but "bring critical plugins under vimpin's jurisdiction and leave the
rest to LazyVim":

| Plugin state                                 | Jurisdiction | Source of truth         |
|----------------------------------------------|--------------|-------------------------|
| Entry exists in `vimpin.toml`                | **vimpin**   | `commit` in the manifest |
| No entry in `vimpin.toml`                    | **LazyVim**  | `lazy-lock.json`        |

Start with 5–10 entries (treesitter, LSP, completion, anything you've
been bitten by) and grow only when needed. The `vimpin adopt/disown/status`
roadmap commands will formalise this workflow; until then, edit
`vimpin.toml` directly and run `vimpin pin` to fill in commits.

In `lua/config/lazy.lua`:

```lua
require("lazy").setup({
  { "LazyVim/LazyVim", import = "lazyvim.plugins", version = "v14.0.0" },
  { import = "plugins._generated" },
})
```

When you override a LazyVim-managed plugin, the pinned commit must remain
compatible with the LazyVim version in use; pinning to a commit that
predates an API LazyVim depends on will break the distribution layer.

## Generated specs (`_generated.lua`)

`vimpin generate` is intended to be **deterministic and reviewable**. The
recommended workflow is:

1. Edit `vimpin.toml`.
2. Run `vimpin pin` (fills in commits).
3. Run `vimpin generate --adapter lazy --output lua/plugins/_generated.lua`.
4. **Commit both files** together.
5. In CI, regenerate and `git diff --exit-code` to catch drift.

Treating `_generated.lua` as a build artifact in `.gitignore` is supported,
but it shifts the contract: every machine that runs Neovim needs to run
`vimpin generate` first. Committing the file is simpler and gives diff
review of every commit change.

## Renovate

The companion preset [`gr1m0h/vimpin-renovate-config`](https://github.com/gr1m0h/vimpin-renovate-config) ships ready-to-use custom managers. Add it to your dotfiles' `renovate.json`:

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["github>gr1m0h/vimpin-renovate-config"]
}
```

That bundles managers for `vimpin.toml`, hand-pinned lazy.nvim / packer.nvim Lua specs, vim-plug specs, and (with the bundled bootstrap helper) `lazy-lock.json`. If you only want the vimpin manager:

```json
{ "extends": ["github>gr1m0h/vimpin-renovate-config:vimpin"] }
```

See the preset's README for the full list of sub-presets, recommended
companion config (`dependencyDashboard`, `prConcurrentLimit`,
`schedule`), and known limits.

## Testing

`vimpin` is tested with `go test ./...`. The current focus is the
`internal/manifest`, `internal/resolver`, and `internal/adapter/lazy`
packages (parser, validator, golden-file adapter output). Promotion from
alpha to beta will require an explicit coverage floor (targeting ~75%)
and an `internal/resolver` test suite that exercises tag/branch resolution
against an `httptest`-backed git fixture.

Contributions to broaden the test matrix are welcome.

## Non-goals

* Replacing plugin managers — keep using `lazy.nvim`, `vim-plug`, etc.
* Managing lazy-load configuration (event/cmd/keys) — adapter outputs only the pin.
* Cryptographic verification of commit contents — planned for a later phase via sigstore.

## License

MIT
