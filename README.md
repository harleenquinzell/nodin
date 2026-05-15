# nodin

[![CI](https://github.com/harleenquinzell/nodin/actions/workflows/ci.yml/badge.svg)](https://github.com/harleenquinzell/nodin/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/harleenquinzell/nodin)](https://goreportcard.com/report/github.com/harleenquinzell/nodin)
[![Go Version](https://img.shields.io/github/go-mod/go-version/harleenquinzell/nodin)](go.mod)
[![License: MIT](https://img.shields.io/github/license/harleenquinzell/nodin)](LICENSE)

A CLI for syncing Notion pages to and from local markdown files.

I don't want to get out of the terminal, so Notion is not my usual go to doc tool, but sometimes we don't have control over that.

I tried other notion sync tools out there, but none that fit my ways of working, as they usually assume a winner when there's conflicts, and the other edit is silently lost. Nodin does a three-way merge, using the last-synced snapshot as the base and writing `<<<<<<<` conflict markers to the file when it can't resolve things automatically.

## Install

```sh
go install github.com/harleenquinzell/nodin/cmd/nodin@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. Or build from source:

```sh
git clone https://github.com/harleenquinzell/nodin
cd nodin
go build -o nodin ./cmd/nodin
```

Requires Go 1.22+ and `git` (only needed for auto-commit, which you can turn off).

## Getting started

```sh
cd ~/my-notion-workspace
nodin init      # prompts for token and root page, writes .nodin.toml here
nodin doctor    # checks your config and connectivity
nodin pull      # sync Notion → local
```

nodin looks for `.nodin.toml` starting in the current directory and walking up, so you can have multiple independent workspaces:

```
~/work/docs/          ← nodin init here → syncs work Notion
~/personal/notes/     ← nodin init here → syncs personal Notion
```

You can also set `NODIN_TOKEN` and `NODIN_ROOT_PAGE_ID` as env vars, or use `--config` to point at a specific file.

## Config

`nodin init` writes a `.nodin.toml` with sensible defaults; edit it to taste and run `nodin doctor` to validate. A user-wide config can also live at `~/.config/nodin/config.toml`. The token can be inlined as `token = "..."` or pointed at a file with `token_file = "~/.secrets/notion-token"`.

## Development

`make help` lists the targets. Integration tests need `.env` (copy from `.env.example`).

If you want to contribute but don't know with what, I keep a list of improvements I want to make in the Issues, check it out!

See [CONTRIBUTING.md](CONTRIBUTING.md) for more.

## License

MIT License
See [LICENSE](LICENSE) for more.
