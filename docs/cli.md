# xhist CLI Specification

**Version:** 2
**Status:** Current

## Overview

The xhist CLI is the **exclusive interface** for AI agents to interact with Excel files. The agent never opens the `.xlsx` directly. Every read and write flows through xhist, which proxies the Excel operation and appends a record to a workspace-level binary log.

One `.xhist` file tracks all Excel files within a workspace. Humans use the same CLI for inspection and debugging.

## Conventions

| Rule | Detail |
|------|--------|
| Output format | JSON to stdout (default). `--human` flag for tabular output. |
| Errors | Stderr, exit code 1. JSON object: `{"error": "message"}` |
| Success | Stdout, exit code 0 |
| Streaming | `--follow` where supported. NDJSON (one JSON object per line). |
| Workspace discovery | Commands automatically find the nearest `{name}.xhist` file by walking up from the current directory. |
| Global flags | `--workspace <path>` explicitly sets the `.xhist` file to use. |
| v1 Compatibility | Read-only commands work with v1 files via `--workspace`. Write commands refuse v1 files. |

## Cell Addressing

Standard Excel notation. Sheet name is delimited by `!`.

| Example | Meaning |
|---------|---------|
| `Sheet1!A1` | Single cell on Sheet1 |
| `Sheet1!A1:C10` | Rectangular range on Sheet1 |
| `A1` | Single cell on the first sheet |
| `A1:C10` | Range on the first sheet |

## Value Encoding (Input/Output)

Cell values in JSON follow these type conventions:

| JSON Type | Excel Type | Example |
|-----------|------------|---------|
| `string` | Text | `"Revenue"` |
| `number` | Number | `42`, `3.14` |
| `boolean` | Boolean | `true` |
| `null` | Empty cell | `null` |
| `string` starting with `=` | Formula | `"=SUM(A1:A10)"` |

Ranges are represented as **row-major arrays of arrays**:

```json
[
  ["Name", "Q1", "Q2"],
  ["Revenue", 1000, 2000],
  ["Cost", 500, 800]
]
```

## Commands

---

### `xhist init`

Create a new workspace `.xhist` file.

```
xhist init [name] [flags]
```

| Flag | Description |
|------|-------------|
| `--agent <name>` | Write `agent.name` metadata record |
| `--model <id>` | Write `agent.model` metadata record |
| `--session <id>` | Write `session.id` metadata record |
| `--force` | Overwrite existing `.xhist` file |

- `[name]` defaults to the current directory name.
- Creates `{name}.xhist` in the current directory.

---

### `xhist read`

Read cells from Excel. Logs a READ op to the workspace history.

```
xhist read <file.xlsx> <range> [flags]
```

| Flag | Description |
|------|-------------|
| `-m, --message <text>` | Why this read was performed (optional) |
| `--no-log` | Read without logging (e.g. internal tooling use) |

**Output:** JSON array of arrays (row-major).

```
$ xhist read budget.xlsx Sheet1!A1:C3
[["Revenue","Cost","Profit"],[1000,500,500],[2000,800,1200]]
```

---

### `xhist write`

Write cells to Excel. Logs a WRITE op to the workspace history.

```
xhist write <file.xlsx> <range> [value] [flags]
```

| Flag | Description |
|------|-------------|
| `-m, --message <text>` | **Required.** Why this write was performed. |
| `--json <json>` | Values as JSON array of arrays |
| `-f, --file <path>` | Read values from a JSON file |
| `--stdin` | Read values from stdin |

**Single cell** — value as positional argument:

```
$ xhist write budget.xlsx Sheet1!A1 "Revenue" -m "Adding header"
```

**Exit output** on success:

```json
{"seq": 1, "cells_written": 2}
```

---

### `xhist log`

Show operation history for the workspace or a specific file.

```
xhist log [file.xlsx] [flags]
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Filter by target file (alternative to positional arg) |
| `--follow` | Stream new ops in real-time (NDJSON) |
| `--sheet <name>` | Filter by sheet name |
| `--action <read\|write\|comment\|comment_set\|comment_get\|comment_delete>` | Filter by action type. `comment` is shorthand for all three `comment_*` variants. |
| `--since <timestamp>` | Ops after this time (ISO 8601 or Unix ms) |
| `--last <n>` | Show only the last N ops |
| `--with-values` | Include cell values in output |
| `--human` | Human-readable table format |

---

### `xhist show`

Show full details of a specific operation by sequence number.

```
xhist show <seq>
```

**Output:**

```json
{
  "seq": 1,
  "target": "budget.xlsx",
  "ts": "2024-01-15T10:30:00Z",
  "action": "write",
  "sheet": "Sheet1",
  "range": "A1:C1",
  "message": "Adding headers",
  "values": [["Revenue", "Cost", "Profit"]]
}
```

---

### `xhist confuse`

Record spreadsheet confusion and resolution events for reconciliation review.

```
xhist confuse raise <file.xlsx> <cell> --archetype <type> --headline <text> --description <text> --evidence <json-array> [flags]
xhist confuse export [flags]
xhist confuse resolve <id> --confidence <high|medium|low> --reasoning <text> [flags]
xhist confuse skip <id> --reasoning <text> [flags]
```

| Subcommand | Required flags | Optional flags |
|------------|----------------|----------------|
| `raise` | `--archetype`, `--headline`, `--description`, `--evidence` | `--payload`, `--dest-table`, `--source-id`, `--message`/`-m`, `--id` |
| `export` | (none) | `--format reconciliation`, `--dest-table`, `--run-id` |
| `resolve` | `--confidence`, `--reasoning`/`-r` | `--value`, `--source`, `--message`/`-m` |
| `skip` | `--reasoning`/`-r` | `--source`, `--message`/`-m` |

`--evidence` must be a non-empty JSON array. Each evidence entry must include a source reference with a source id and a concrete locator, using either camelCase or snake_case keys:

```json
[
  {
    "id": "ev-sheet-a1",
    "sourceRef": {
      "sourceId": "budget.xlsx",
      "locator": { "kind": "xlsx", "sheet": "Sheet1", "range": "A1" }
    },
    "tone": "focus",
    "label": "Sheet1 A1"
  }
]
```

Locator requirements:

- PDF evidence: `{"kind":"pdf","page":1,"bbox":{"left":0.1,"top":0.2,"width":0.3,"height":0.04}}`
- XLSX evidence: `{"kind":"xlsx","sheet":"Sheet1","range":"A1:B2"}`
- Warehouse/database evidence: `{"kind":"warehouse","rowKey":{"account_code":"40110"},"column":"amount"}`
- Text fallback: `{"kind":"text","start":120,"end":180}`

For gaps, cite the blank or expected source region. For conflicts or drift, include one evidence entry for each disagreeing source value.

`xhist confuse export` includes this evidence in both the exported reconciliation item and payload so downstream review surfaces can render the source context.

---

### `xhist comment`

Manage native Excel cell comments. Three subcommands: `set`, `get`, `delete`. Comments are written into the `.xlsx` file as real Excel notes AND recorded in the workspace log as `comment_set` / `comment_delete` ops.

```
xhist comment set <file.xlsx> <range> [text] [flags]
xhist comment get <file.xlsx> <range>
xhist comment delete <file.xlsx> <range> [flags]
```

| Subcommand | Required flags | Optional flags |
|------------|----------------|----------------|
| `set` | `--message`/`-m <why>` | `--json <grid>`, `--author <name>` |
| `get` | (none) | (none) |
| `delete` | `--message`/`-m <why>` | (none) |

**Set a single cell's comment** — provide the text as a positional arg:

```bash
xhist comment set budget.xlsx 'Sheet1!A1' "Source: 2024 10-K p.47" -m "Citation" --author agent-1
```

**Set a range of comments** — use `--json` with a 2D grid of strings (empty string = skip that cell):

```bash
xhist comment set budget.xlsx 'Sheet1!A1:B2' \
  --json '[["Source: SAP",""],["Source: Oracle","Source: Bloomberg"]]' \
  -m "Adding citations" --author agent-1
```

**Get comments** — returns `null` / `{}` if none, a single object for single-cell queries, or a sparse array for ranges.

**Delete comments** — removes both the in-workbook note and records a `comment_delete` op.

---

### `xhist info`

Show metadata and stats about the workspace history.

```
xhist info [file.xlsx]
```

| Flag | Description |
|------|-------------|
| `--file <path>` | Filter stats to a specific file |

**Output:**

```json
{
  "workspace": "project-alpha",
  "created": "2024-01-15T10:30:00Z",
  "ops": 150,
  "files": [
    {"path": "budget.xlsx", "ops": 42},
    {"path": "forecast.xlsx", "ops": 108}
  ],
  "last_op": "2024-01-15T11:45:00Z",
  "metadata": {
    "agent.name": "excel-agent"
  }
}
```

---

### `xhist state`

Reconstruct the known spreadsheet state by replaying WRITE ops.

```
xhist state <file.xlsx> [flags]
```

| Flag | Description |
|------|-------------|
| `--sheet <name>` | Show a specific sheet only |
| `--at <seq>` | Reconstruct state as of operation N |
| `--range <range>` | Show a specific range only |
| `--diff` | Compare reconstructed state vs actual Excel file |

---

### `xhist migrate`

Migrate legacy v1 `.xhist` files into a new workspace log.

```
xhist migrate [--name NAME] [--dir DIR] [--dry-run]
```

- Scans `DIR` (default `.`) for `.xhist` files.
- Creates a new workspace log `NAME.xhist`.
- Imports all records, preserving timestamps and sequence order.

---

### `xhist verify`

Check integrity of the workspace `.xhist` file.

```
xhist verify
```

---

### `xhist reindex`

Rebuild the sidecar index for the workspace.

```
xhist reindex
```

---

### `xhist repair`

Truncate a corrupted workspace `.xhist` file.

```
xhist repair [flags]
```

---

### `xhist sheets`

List sheets in an Excel file. Reads the `.xlsx` directly.

```
xhist sheets <file.xlsx>
```

---

## Agent Integration Patterns

### Recommended Agent Workflow

```
1. xhist init project-name                → setup workspace
2. xhist sheets budget.xlsx               → discover structure
3. xhist read budget.xlsx Sheet1!A1:Z1    → read headers
4. xhist write budget.xlsx Sheet1!E1 "Growth" -m "Adding column"
5. xhist log budget.xlsx --last 5         → review recent operations
```

### Resuming After Context Loss

```
1. xhist info                             → see workspace overview
2. xhist log --last 20                    → recall global timeline
3. xhist log budget.xlsx --last 5         → focus on specific file
4. ... continue work ...
```
