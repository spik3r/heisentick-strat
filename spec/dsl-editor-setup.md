# Editing Strat Strategies in Your Own Editor

Strat has standalone sources in `strategies/source/*.strat` (source
of truth — `pnpm run dsl:embed` regenerates the JS literals; CI fails on
drift) and a zero-dependency LSP server so external editors get the same
compiler diagnostics as the in-app editor.

## The LSP server

```bash
pnpm run dsl:lsp        # speaks LSP over stdio; started by your editor
```

Capabilities: full-document sync with `publishDiagnostics` on open/change
(line/column-accurate, same messages as `pnpm run dsl:lint`), and keyword
completion for sections, directive heads, and level names sourced from the
parser's own vocabulary (`strat/implementations/browser-runtime/compiler/directiveHeads.js`).

The server is `scripts/dsl/dslLsp.mjs`. It must run with the repo as the working
directory so the `#engine/*` import alias resolves.

## Neovim

```lua
-- ~/.config/nvim/after/ftdetect/strat.lua
vim.filetype.add({ extension = { strat = 'strat' } })

-- ~/.config/nvim/after/ftplugin/strat.lua (or anywhere in your config)
vim.api.nvim_create_autocmd('FileType', {
  pattern = 'strat',
  callback = function(args)
    vim.lsp.start({
      name = 'backtester-strat',
      cmd = { 'pnpm', 'run', '--silent', 'dsl:lsp' },
      root_dir = vim.fs.root(args.buf, { 'package.json', '.git' }),
    })
  end,
})
```

Open any `strategies/source/*.strat` file: parse errors appear as
diagnostics on the exact source line, and `<C-x><C-o>` /
your completion plugin offers Strat keywords.

After editing, sync and validate:

```bash
pnpm run dsl:embed          # regenerate the JS literal from the .strat file
pnpm run dsl:lint           # compile every registered spec
pnpm run dsl:embed:check    # what CI runs; verifies .strat and JS agree
```

Or run the watch loop, which does the compile + re-embed on every save and can
run a quick backtest when the spec is clean (needs local market data):

```bash
pnpm run dsl:watch -- --file=strategies/source/<name>.strat
pnpm run dsl:watch -- --file=/tmp/scratch.strat --report=1 --symbol=XAUUSD --tf=1h
```

`--once=1` does a single compile pass and exits non-zero on errors (useful as a
save hook or pre-commit check).

## VS Code

Any generic LSP client extension works; point it at
`pnpm run --silent dsl:lsp` with the repo root as cwd and the `strat` language
id mapped to `*.strat`.

## Roadmap

The pop-out browser editor tab, Lezer highlighting, and a `dsl:watch`
save-to-chart loop are tracked in `plans/COMPLETED.md` (editor/LSP closeout)
(slices 3, 4, 6). If the Go parser from
the Go parser (shipped; see the Strat/Go closeout in `plans/COMPLETED.md`) is adopted here, the server binary can
swap behind this same client setup.
