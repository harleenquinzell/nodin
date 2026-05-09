<!--
Thanks for the contribution. Please fill in the sections below — the more
context you give, the faster I can review.

If your PR is from a fork, integration and e2e tests will skip in CI (GitHub
doesn't expose secrets to fork workflows). I'll pull your changes into a branch
in this repo to run the full suite before merging.
-->

## Summary

<!-- One or two sentences: what does this change do, and why? -->

## Linked issue

<!-- "Closes #123" or "Refs #45". Delete this section if there's no related issue. -->

## Type of change

<!-- Tick the box(es) that apply. -->

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor (no behavior change)
- [ ] Docs / tooling / CI
- [ ] Breaking change

## How was this tested?

<!--
Briefly: what did you run, and what did you observe?
Examples:
  - "Added unit test in internal/convert/pull_test.go covering the new block type."
  - "make test-integration locally; saw 0 failures."
  - "Manually pulled my workspace and verified the rendered output matches."
-->

## Checklist

- [ ] `make test` passes locally
- [ ] New code paths have at least one test
- [ ] No tokens, secrets, or workspace IDs are committed (check diffs and logs)
- [ ] Commit messages follow `area: short imperative` (areas: convert, notion, sync, cli, state, merge, docs, ci)
- [ ] CHANGELOG.md updated under `## [Unreleased]` if this is user-visible

## Notes for the reviewer

<!-- Anything tricky to look at, design tradeoffs you considered, follow-up work you're punting on. Optional. -->
