# vimpin

A universal version pinner for Vim/Neovim plugins.

`vimpin` is to Vim/Neovim plugins what [pinact](https://github.com/suzuki-shunsuke/pinact) is to GitHub Actions: it pins every plugin to an explicit commit hash via a TOML manifest, and integrates with [Renovate](https://docs.renovatebot.com/) so updates flow through reviewable pull requests instead of silent `:Lazy update` calls.

> **Status:** alpha, used by author. Schema may change.

## Why

`lazy.nvim`'s lockfile only *records* the current commit — it does not *lock* it. `:Lazy update` will happily move every plugin to a new commit the moment you forget to pin specs explicitly. For supply-chain-conscious setups this is unacceptable.

`vimpin` makes the pin the source of truth and reduces the lockfile to a derived artifact.

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

Resolution precedence is **commit > tag > branch** across three axes:

1. **Install**: whatever ref is highest-priority among those present is what gets checked out.
2. **Renovate**: when both are present, tag tracking is preferred over branch HEAD tracking.
3. **Add**: `vimpin add owner/repo` defaults to tag if a release exists, else default branch.

## Commands

```text
vimpin pin                  Resolve tag/branch → commit, write back
  --refresh                 Re-resolve entries that already have a commit
vimpin verify               Confirm every entry is pinned to a 40-hex commit
  --strict                  Also re-check tag/branch ↔ commit alignment
vimpin generate             Emit plugin-manager-specific spec
  --adapter lazy            (default) lazy.nvim spec
  --groups core,work        Filter by group
  --output PATH             Write to file (default stdout)
```

## LazyVim integration

vimpin and LazyVim live in two layers:

* **LazyVim distribution layer** — the LazyVim spec itself is pinned (by `version` or `commit`); its transitives are recorded in `lazy-lock.json`.
* **vimpin manifest layer** — your own plugins and any explicit overrides of LazyVim-managed plugins.

In `lua/config/lazy.lua`:

```lua
require("lazy").setup({
  { "LazyVim/LazyVim", import = "lazyvim.plugins", version = "v14.0.0" },
  { import = "plugins._generated" },
})
```

Overriding a LazyVim plugin is a power tool. Use `layer = "override"` only when you genuinely need to deviate from LazyVim's chosen version — manifest entries on LazyVim-managed plugins can break LazyVim if the pinned commit predates an API LazyVim depends on.

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

See the preset's README for the full list of sub-presets and known limits.

## Non-goals

* Replacing plugin managers — keep using `lazy.nvim`, `vim-plug`, etc.
* Managing lazy-load configuration (event/cmd/keys) — adapter outputs only the pin.
* Cryptographic verification of commit contents — planned for a later phase via sigstore.

## License

MIT
