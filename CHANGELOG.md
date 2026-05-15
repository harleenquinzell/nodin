# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `new-db` command: interactive creation of Notion databases with schema prompts
- `formula` and `relation` property types for `new-db` schemas
- `status`, `people`, `files`, `created_time`, `last_edited_time`, `created_by`, `last_edited_by`, `unique_id` property types
- Shell completion command (`nodin completion bash|zsh|fish|powershell`)
- Push creates pages in Notion if they don't exist locally yet
- Toggle / callout block support in pull
- Index is read once per pull/push instead of repeatedly

[Unreleased]: https://github.com/harleenquinzell/nodin/compare/HEAD...HEAD
