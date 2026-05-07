#!/usr/bin/env python3
"""
seed-notion.py — create a fresh nodin test-fixture sandbox in Notion.

Each run creates a new top-level page named `nodin-fixtures-<timestamp>` under
the chosen parent, and populates it with:

  - A 3-level nested page tree (Projects/Alpha/Notes)
  - A "Block sampler" page exercising every supported block type
  - A "Tasks" database with select/multi-select/date/checkbox/number/url props
    and 5 seed rows
  - A page with an external image and a bookmark (asset path exercise)

Reads tokens from .env in the current directory or from the environment.
Defaults to NOTION_SPACE_TOKEN; override with --token-env.

Usage
  python3 scripts/seed-notion.py                    # auto-pick parent
  python3 scripts/seed-notion.py --parent <page-id>
  python3 scripts/seed-notion.py --token-env NOTION_WORKSPACE_TOKEN
  python3 scripts/seed-notion.py --dry-run          # print plan, don't call API
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from pathlib import Path

API = "https://api.notion.com/v1"
NOTION_VERSION = "2022-06-28"


# ---------- env / http ----------

def load_env(path: Path) -> None:
    if not path.exists():
        return
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        k, _, v = line.partition("=")
        os.environ.setdefault(k.strip(), v.strip().strip('"').strip("'"))


def http(method: str, path: str, token: str, body: dict | None = None) -> dict:
    url = f"{API}{path}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Notion-Version", NOTION_VERSION)
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return json.loads(r.read())
    except urllib.error.HTTPError as e:
        sys.stderr.write(f"\n[HTTP {e.code}] {method} {path}\n")
        sys.stderr.write(e.read().decode(errors="replace") + "\n")
        raise


# ---------- block builders ----------

def rt(text: str, *, bold=False, italic=False, code=False, link: str | None = None) -> dict:
    item = {"type": "text", "text": {"content": text}}
    if link:
        item["text"]["link"] = {"url": link}
    ann = {}
    if bold: ann["bold"] = True
    if italic: ann["italic"] = True
    if code: ann["code"] = True
    if ann:
        item["annotations"] = ann
    return item


def heading(level: int, text: str) -> dict:
    key = f"heading_{level}"
    return {"object": "block", "type": key, key: {"rich_text": [rt(text)]}}


def paragraph(rich) -> dict:
    if isinstance(rich, str):
        rich = [rt(rich)]
    return {"object": "block", "type": "paragraph", "paragraph": {"rich_text": rich}}


def bullet(text: str) -> dict:
    return {"object": "block", "type": "bulleted_list_item",
            "bulleted_list_item": {"rich_text": [rt(text)]}}


def numbered(text: str) -> dict:
    return {"object": "block", "type": "numbered_list_item",
            "numbered_list_item": {"rich_text": [rt(text)]}}


def todo(text: str, checked: bool = False) -> dict:
    return {"object": "block", "type": "to_do",
            "to_do": {"rich_text": [rt(text)], "checked": checked}}


def toggle(text: str, children: list[dict]) -> dict:
    return {"object": "block", "type": "toggle",
            "toggle": {"rich_text": [rt(text)], "children": children}}


def quote(text: str) -> dict:
    return {"object": "block", "type": "quote", "quote": {"rich_text": [rt(text)]}}


def callout(text: str, emoji: str = "💡") -> dict:
    return {"object": "block", "type": "callout",
            "callout": {"rich_text": [rt(text)], "icon": {"type": "emoji", "emoji": emoji}}}


def code(text: str, language: str = "python") -> dict:
    return {"object": "block", "type": "code",
            "code": {"rich_text": [rt(text)], "language": language}}


def divider() -> dict:
    return {"object": "block", "type": "divider", "divider": {}}


def bookmark(url: str, caption: str = "") -> dict:
    block = {"object": "block", "type": "bookmark", "bookmark": {"url": url}}
    if caption:
        block["bookmark"]["caption"] = [rt(caption)]
    return block


def image_external(url: str, caption: str = "") -> dict:
    block = {"object": "block", "type": "image",
             "image": {"type": "external", "external": {"url": url}}}
    if caption:
        block["image"]["caption"] = [rt(caption)]
    return block


def equation(expr: str) -> dict:
    return {"object": "block", "type": "equation", "equation": {"expression": expr}}


def table(rows: list[list[str]], has_header: bool = True) -> dict:
    width = max(len(r) for r in rows)
    children = []
    for row in rows:
        cells = [[rt(c)] for c in row]
        while len(cells) < width:
            cells.append([rt("")])
        children.append({"object": "block", "type": "table_row",
                         "table_row": {"cells": cells}})
    return {"object": "block", "type": "table",
            "table": {"table_width": width, "has_column_header": has_header,
                      "has_row_header": False, "children": children}}


# ---------- creation helpers ----------

def create_page(token: str, parent_id: str, title: str,
                children: list[dict] | None = None,
                emoji: str | None = None) -> dict:
    body = {
        "parent": {"page_id": parent_id},
        "properties": {"title": {"title": [rt(title)]}},
    }
    if emoji:
        body["icon"] = {"type": "emoji", "emoji": emoji}
    if children:
        body["children"] = children
    return http("POST", "/pages", token, body)


def append_blocks(token: str, parent_block_id: str, blocks: list[dict]) -> dict:
    return http("PATCH", f"/blocks/{parent_block_id}/children", token,
                {"children": blocks})


# ---------- fixture content ----------

def block_sampler() -> list[dict]:
    return [
        heading(1, "Block sampler"),
        paragraph("This page exercises every block type nodin needs to round-trip."),
        divider(),

        heading(2, "Headings"),
        heading(3, "A level-three heading"),
        paragraph("Paragraph after a heading-3."),

        heading(2, "Inline formatting"),
        paragraph([
            rt("This paragraph mixes "),
            rt("bold", bold=True), rt(", "),
            rt("italic", italic=True), rt(", "),
            rt("inline code", code=True), rt(", and a "),
            rt("link", link="https://developers.notion.com/"),
            rt("."),
        ]),

        heading(2, "Lists"),
        bullet("First bullet point"),
        bullet("Second bullet"),
        bullet("Third bullet with more text to test wrapping behaviour"),
        numbered("First numbered item"),
        numbered("Second numbered item"),
        numbered("Third numbered item"),

        heading(2, "To-do"),
        todo("Unchecked task", checked=False),
        todo("Already done", checked=True),
        todo("Another open one", checked=False),

        heading(2, "Toggle"),
        toggle("Click to expand", [
            paragraph("Hidden paragraph inside the toggle."),
            bullet("Nested bullet inside toggle"),
        ]),

        heading(2, "Quote & callout"),
        quote("\"The future is already here — it's just not very evenly distributed.\""),
        callout("Heads up: callouts carry an emoji that nodin must round-trip.", emoji="⚠️"),

        heading(2, "Code"),
        code(
            'def greet(name: str) -> str:\n    return f"Hello, {name}!"\n\nprint(greet("nodin"))',
            language="python",
        ),
        code(
            'package main\n\nimport "fmt"\n\nfunc main() {\n    fmt.Println("hello")\n}',
            language="go",
        ),

        heading(2, "Equation"),
        equation("e^{i\\pi} + 1 = 0"),

        heading(2, "Table"),
        table([
            ["Block", "Status", "Notes"],
            ["paragraph", "lossless", "—"],
            ["callout", "lossy", "blockquote + emoji"],
            ["toggle", "lossy", "<details> in markdown"],
        ], has_header=True),

        divider(),
        paragraph("End of sampler."),
    ]


def create_tasks_database(token: str, parent_id: str) -> dict:
    body = {
        "parent": {"type": "page_id", "page_id": parent_id},
        "title": [rt("Tasks")],
        "icon": {"type": "emoji", "emoji": "✅"},
        "properties": {
            "Title": {"title": {}},
            "Status": {
                "select": {"options": [
                    {"name": "To Do",       "color": "gray"},
                    {"name": "In Progress", "color": "yellow"},
                    {"name": "Done",        "color": "green"},
                    {"name": "Blocked",     "color": "red"},
                ]}
            },
            "Tags": {
                "multi_select": {"options": [
                    {"name": "infra",   "color": "blue"},
                    {"name": "api",     "color": "purple"},
                    {"name": "docs",    "color": "orange"},
                    {"name": "polish",  "color": "pink"},
                    {"name": "bug",     "color": "red"},
                ]}
            },
            "Priority": {"number": {"format": "number"}},
            "Due":      {"date": {}},
            "Done":     {"checkbox": {}},
            "Link":     {"url": {}},
            "Notes":    {"rich_text": {}},
        },
    }
    return http("POST", "/databases", token, body)


def seed_tasks(token: str, db_id: str) -> list[str]:
    today = datetime.now(timezone.utc).date()
    rows = [
        {
            "Title": "Wire up Notion API client",
            "Status": "In Progress", "Tags": ["infra", "api"],
            "Priority": 1, "Due": today + timedelta(days=2), "Done": False,
            "Link": "https://developers.notion.com/reference/intro",
            "Notes": "Token-bucket limiter, exp backoff, paginated cursors.",
        },
        {
            "Title": "Block converter — paragraphs & headings",
            "Status": "Done", "Tags": ["api"],
            "Priority": 2, "Due": today - timedelta(days=1), "Done": True,
            "Link": "", "Notes": "Round-trip covered by unit fixture.",
        },
        {
            "Title": "Three-way merge prototype",
            "Status": "To Do", "Tags": ["infra"],
            "Priority": 1, "Due": today + timedelta(days=7), "Done": False,
            "Link": "", "Notes": "git merge-file shellout first, pure-Go later.",
        },
        {
            "Title": "Asset download path",
            "Status": "Blocked", "Tags": ["api", "bug"],
            "Priority": 3, "Due": today + timedelta(days=10), "Done": False,
            "Link": "https://developers.notion.com/reference/block#image",
            "Notes": "Signed URLs expire ~1h; need re-fetch retry.",
        },
        {
            "Title": "Write README & brew formula",
            "Status": "To Do", "Tags": ["docs", "polish"],
            "Priority": 4, "Due": today + timedelta(days=21), "Done": False,
            "Link": "", "Notes": "Phase 04 deliverable.",
        },
    ]

    created = []
    for r in rows:
        props = {
            "Title":    {"title": [rt(r["Title"])]},
            "Status":   {"select": {"name": r["Status"]}},
            "Tags":     {"multi_select": [{"name": t} for t in r["Tags"]]},
            "Priority": {"number": r["Priority"]},
            "Due":      {"date": {"start": r["Due"].isoformat()}},
            "Done":     {"checkbox": r["Done"]},
            "Notes":    {"rich_text": [rt(r["Notes"])] if r["Notes"] else []},
        }
        if r["Link"]:
            props["Link"] = {"url": r["Link"]}
        page = http("POST", "/pages", token, {
            "parent": {"database_id": db_id},
            "properties": props,
        })
        created.append(page["id"])
    return created


def media_page() -> list[dict]:
    return [
        heading(1, "Media & references"),
        paragraph("This page exercises external assets — images and bookmarks."),
        heading(2, "External image"),
        image_external(
            "https://placehold.co/600x400/png?text=nodin+test+image",
            caption="Placeholder image served from placehold.co",
        ),
        heading(2, "Bookmark"),
        bookmark("https://developers.notion.com/", caption="Notion API reference"),
        paragraph("End of media page."),
    ]


# ---------- main orchestration ----------

def pick_parent(token: str) -> tuple[str, str]:
    """Pick a sensible parent page from the integration's accessible pages."""
    r = http("POST", "/search", token, {"page_size": 50,
                                         "filter": {"value": "page", "property": "object"}})
    candidates = [p for p in r.get("results", [])
                  if p.get("parent", {}).get("type") == "workspace"]
    if not candidates:
        candidates = r.get("results", [])
    if not candidates:
        sys.stderr.write(
            "No accessible pages. Share a page with this integration first.\n"
            "  Notion → page → ••• → Connections → add this integration.\n")
        sys.exit(2)
    # Prefer a "Teamspace Home" / "Home" if present; else first candidate.
    def title_of(p):
        for v in p.get("properties", {}).values():
            if v.get("type") == "title" and v.get("title"):
                return "".join(t.get("plain_text", "") for t in v["title"])
        return ""
    preferred = [p for p in candidates if title_of(p).lower() in
                 ("teamspace home", "home")]
    chosen = preferred[0] if preferred else candidates[0]
    return chosen["id"], title_of(chosen) or "(untitled)"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--token-env", default="NOTION_SPACE_TOKEN",
                    help="env var holding the integration token (default: %(default)s)")
    ap.add_argument("--parent", default=None,
                    help="parent page ID (default: auto-detect from /v1/search)")
    ap.add_argument("--name", default=None,
                    help="root sandbox page name (default: nodin-fixtures-<timestamp>)")
    ap.add_argument("--dry-run", action="store_true",
                    help="print the plan without calling the API")
    args = ap.parse_args()

    load_env(Path(".env"))
    token = os.environ.get(args.token_env)
    if not token:
        sys.stderr.write(f"missing env var: {args.token_env}\n")
        return 2

    if args.dry_run:
        print("[dry-run] would auth, pick parent, create sandbox + 4 sub-pages + 1 db")
        return 0

    me = http("GET", "/users/me", token)
    bot = me.get("name", "?")
    workspace = me.get("bot", {}).get("workspace_name", "?")
    print(f"auth ok: bot={bot!r} workspace={workspace!r}")

    parent_id, parent_title = (args.parent, "(supplied)") if args.parent \
        else pick_parent(token)
    print(f"parent: {parent_title!r}  id={parent_id}")

    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d-%H%M%S")
    root_name = args.name or f"nodin-fixtures-{ts}"

    # 1. Sandbox root
    root = create_page(token, parent_id, root_name, emoji="🧪", children=[
        heading(1, root_name),
        paragraph(f"Generated {datetime.now(timezone.utc).isoformat(timespec='seconds')}."),
        paragraph("Pages, databases and assets below are throwaway test data for nodin."),
        divider(),
        bullet("Block sampler — every supported block type"),
        bullet("Projects / Alpha / Notes — 3-level nested hierarchy"),
        bullet("Tasks — database with mixed property types"),
        bullet("Media — external image and bookmark"),
    ])
    root_id = root["id"]
    print(f"  + sandbox root           {root_id}  {root_name}")

    # 2. Block sampler
    sampler = create_page(token, root_id, "Block sampler", emoji="🧱",
                          children=block_sampler())
    print(f"  + block sampler          {sampler['id']}")

    # 3. Nested page tree: Projects → Alpha → Notes
    projects = create_page(token, root_id, "Projects", emoji="📁", children=[
        heading(1, "Projects"),
        paragraph("Top of a 3-level page hierarchy."),
    ])
    alpha = create_page(token, projects["id"], "Alpha", emoji="🅰️", children=[
        heading(1, "Project Alpha"),
        paragraph("A child of Projects. Sub-page below."),
    ])
    notes = create_page(token, alpha["id"], "Notes", emoji="📝", children=[
        heading(1, "Alpha — Notes"),
        paragraph("Deeply nested page; tests recursive traversal."),
        bullet("Decision: ship pull before push"),
        bullet("Risk: block-ID anchors break on heavy reordering"),
        callout("Three-way merge depends on the snapshot file existing.", emoji="🧭"),
    ])
    print(f"  + Projects               {projects['id']}")
    print(f"  +   Projects/Alpha       {alpha['id']}")
    print(f"  +     Projects/Alpha/Notes {notes['id']}")

    # 4. Tasks database with seed rows
    db = create_tasks_database(token, root_id)
    print(f"  + Tasks (database)       {db['id']}")
    rows = seed_tasks(token, db["id"])
    print(f"    seeded {len(rows)} task rows")

    # 5. Media page
    media = create_page(token, root_id, "Media", emoji="🖼", children=media_page())
    print(f"  + Media                  {media['id']}")

    print()
    print(f"DONE — root page: https://www.notion.so/{root_id.replace('-', '')}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
