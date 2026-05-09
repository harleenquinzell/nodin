# Security policy

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

To report a vulnerability, use [GitHub's private vulnerability reporting](https://github.com/harleenquinzell/nodin/security/advisories/new). This sends the report directly to me without making it public.

I will prioritize this type of issue, as otherwise I wouldn't look at it.

## Supported versions

Only the latest released version is supported. There is no LTS branch. If a vulnerability is found, the fix will land on `main` and be cut as a new patch release.

## Scope

In scope — please report:

- **Token leakage**: any code path that writes a Notion token to logs, error messages, on-disk state, or the auto-commit history.
- **Path traversal or unsafe file writes**: anything where Notion-supplied data could cause writes outside the configured `sync_dir`.
- **Command injection**: shell-callable inputs (e.g. via the auto-commit feature) that could execute attacker-controlled commands.
- **Stale-data attacks**: the three-way merge or snapshot logic accepting forged input that silently overwrites user data.
- **Dependency vulnerabilities** in our direct dependencies that affect nodin's runtime behavior.

Out of scope — please don't report:

- Bugs in Notion's API itself (report those to Notion).
- Issues that require an attacker to already have your token or local filesystem access (those are pre-conditions that nodin can't defend against).
- Theoretical issues without a working proof of concept.

## What to include

A good report has:

1. The version of nodin (`nodin --version`) or the commit SHA you tested against.
2. A minimal reproduction: steps, sample workspace shape, or sample markdown.
3. The impact: what an attacker can do with this.
4. (Optional) A suggested fix.

## After a fix

Once a fix is released, I'll publish a [GitHub security advisory](https://github.com/harleenquinzell/nodin/security/advisories) describing the issue, affected versions, and credit to the reporter (unless you'd prefer to remain anonymous).
