# 0001 — Three-way merge for sync conflicts

Date: 2026-05-15
Status: implemented

## Context

When local and Notion both change a page between syncs, something has to give. The tools I tried before nodin pick a winner — usually whichever side ran most recently — and the other edit just disappears. That's the bug I'm trying not to ship.

## Decision

Keep a snapshot of the last-synced content under `.nodin/` and treat it as the common ancestor. On every pull and push, do a three-way merge of `snapshot` (base) + the local file (mine) + Notion (theirs).

The merge itself shells out to `git merge-file -p --diff3`. Git already knows how to do this well, and most users already have it installed. When it can't auto-resolve, the file is written with diff3-style `<<<<<<<` markers and the sync stops for that page — the user resolves it the same way they'd resolve a git conflict.

## Consequences

- Edits on both sides survive whenever the changes don't overlap.
- `git` becomes a hard dependency for sync, not just for the optional auto-commit. `nodin doctor` calls this out.
- The `.nodin/` snapshot directory is part of the contract. Delete it and the next sync looks like a fresh pull — conflicts won't be detected for pages that already existed.
- Conflict resolution lives in the user's editor, not in nodin. I'm OK with this; it's familiar to the audience and one less surface to design.
