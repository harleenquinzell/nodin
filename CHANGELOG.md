# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Conflict hints after `pull`, `push`, and `status`: when merge conflicts are present, each command now lists the conflicted files with a ready-to-run editor command (`$VISUAL`/`$EDITOR`) that jumps directly to the first conflict marker (e.g. `nvim +42 pages/my-page.md`)
- `diff` shows the same hint at the top when the target file contains unresolved conflict markers

## [0.1.0] - 2026-05-19

### Added
- `new-db` command: interactive creation of Notion databases with schema prompts
- `formula` and `relation` property types for `new-db` schemas
- `status`, `people`, `files`, `created_time`, `last_edited_time`, `created_by`, `last_edited_by`, `unique_id` property types
- Shell completion command (`nodin completion bash|zsh|fish|powershell`)
- Push creates pages in Notion if they don't exist locally yet
- Toggle / callout block support in pull
- Index is read once per pull/push instead of repeatedly
- `update` command: checks latest version and runs `go install` to self-update
- Pull now removes local files for pages moved or deleted in Notion

### Fixed
- Database entries with a title column named something other than "Name" could not be created

[Unreleased]: https://github.com/harleenquinzell/nodin/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/harleenquinzell/nodin/releases/tag/v0.1.0
