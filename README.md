# nodin

A CLI for syncing Notion pages to and from local markdown files.

I don't want to get out of the terminal, so Notion is not my usual go to doc tool, but sometimes we don't have control over that.

I tried other notion sync tools out there, but none that fit my ways of working, as they usually assume a winner when there's conflicts, and the other edit is silently lost. Nodin does a proper three-way merge instead, using the last-synced snapshot as the base and writing `<<<<<<<` conflict markers to the file when it can't resolve things automatically.

## Install

```sh
go install github.com/harleenquinzell/nodin/cmd/nodin@latest
```

Or from source:

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

## Commands

```sh
nodin pull                       # fetch changes from Notion
nodin push                       # push local changes to Notion
nodin pull --page <id>           # pull a single page
nodin push --page <id>           # push a single page
nodin push --dry-run             # see what would be pushed without doing it
nodin pull --since 2024-01-15T00:00:00Z   # override the incremental cursor
nodin status                     # show which files have local changes
nodin diff <file>                # diff a file against its last snapshot
nodin doctor                     # health check
```

## Config file

`~/.config/nodin/config.toml`:

```toml
[auth]
token = "secret_..."        # or use token_file = "~/.secrets/notion-token"

[sync]
root_page_id    = "3589c940-..."
sync_dir        = "~/notion"
rate_limit_rps  = 3          # Notion's API cap
concurrency     = 4
auto_commit     = true       # git commit before/after each sync
download_assets = true       # download images and files locally
```

## Development

```sh
make build           # build the binary
make test            # unit tests, no network needed
make test-integration  # integration tests — reads credentials from .env
make help            # list all targets
```

The Makefile works from any directory. For integration tests, copy `.env.example` to `.env` and fill in your values first.

See [CONTRIBUTING.md](CONTRIBUTING.md) for more.

## License

MIT License
See [LICENSE](LICENSE) for more.
