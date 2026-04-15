# xhist CLI Specification

**Version:** 1
**Status:** Draft

## Overview

The xhist CLI is the **exclusive interface** for AI agents to interact with Excel files. The agent never opens the `.xlsx` directly — every read and write flows through xhist, which proxies the Excel operation and appends a record to the binary log.

Humans use the same CLI for inspection and debugging.

## Conventions

| Rule | Detail |
|------|--------|
| Output format | JSON to stdout (default). `--human` flag for tabular output. |
| Errors | Stderr, exit code 1. JSON object: `{"error": "message"}` |
| Success | Stdout, exit code 0 |
| Streaming | `--follow` where supported. NDJSON (one JSON object per line). |
| Auto-init | If no `.xhist` file exists for the target, create one automatically on first operation. |
| File discovery | `budget.xlsx` → looks for `budget.xhist` in the same directory. |

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

Create a new `.xhist` file for an Excel file. Optional — other commands auto-init.

```
xhist init <file.xlsx> [flags]
```

| Flag | Description |
|------|-------------|
| `--agent <name>` | Write `agent.name` metadata record |
| `--model <id>` | Write `agent.model` metadata record |
| `--session <id>` | Write `session.id` metadata record |
| `--force` | Overwrite existing `.xhist` file |

- If `<file.xlsx>` does not exist, creates an empty workbook.
- If `.xhist` already exists and `--force` is not set, exits with error.

---

### `xhist read`

Read cells from Excel. Logs a READ op to the history.

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

Single cell returns a scalar:

```
$ xhist read budget.xlsx Sheet1!A1
"Revenue"
```

---

### `xhist write`

Write cells to Excel. Logs a WRITE op to the history.

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

**Range** — values as JSON:

```
$ xhist write budget.xlsx Sheet1!A1:B2 --json '[["Name","Value"],["Revenue",1000]]' -m "Adding data"
```

**From stdin:**

```
$ echo '[["Name","Value"],["Revenue",1000]]' | xhist write budget.xlsx Sheet1!A1:B2 --stdin -m "Piped data"
```

Type coercion: JSON strings starting with `=` are written as formulas. `null` clears the cell.

**Exit output** on success (for agent confirmation):

```json
{"seq": 1, "cells_written": 2}
```

---

### `xhist log`

Show operation history.

```
xhist log <file.xlsx> [flags]
```

| Flag | Description |
|------|-------------|
| `--follow` | Stream new ops in real-time (NDJSON) |
| `--sheet <name>` | Filter by sheet name |
| `--action <read\|write>` | Filter by action type |
| `--since <timestamp>` | Ops after this time (ISO 8601 or Unix ms) |
| `--last <n>` | Show only the last N ops |
| `--with-values` | Include cell values in output |
| `--human` | Human-readable table format |

**Default output** (JSON array, values omitted for brevity):

```json
[
  {"seq": 1, "ts": "2024-01-15T10:30:00Z", "action": "write", "sheet": "Sheet1", "range": "A1:C1", "message": "Adding headers"},
  {"seq": 2, "ts": "2024-01-15T10:30:01Z", "action": "read", "sheet": "Sheet1", "range": "A1:C5", "message": "Reviewing data"}
]
```

**Follow mode** (NDJSON, one line per op, blocks until new ops arrive):

```
$ xhist log budget.xlsx --follow
{"seq": 3, "ts": "...", "action": "write", "sheet": "Sheet1", "range": "A2:C2", "message": "Adding Q1 data"}
{"seq": 4, "ts": "...", "action": "write", "sheet": "Sheet1", "range": "A3:C3", "message": "Adding Q2 data"}
```

**Human mode:**

```
$ xhist log budget.xlsx --human
 SEQ  TIME                  ACTION  SHEET   RANGE   MESSAGE
 1    2024-01-15T10:30:00Z  WRITE   Sheet1  A1:C1   Adding headers
 2    2024-01-15T10:30:01Z  READ    Sheet1  A1:C5   Reviewing data
```

---

### `xhist show`

Show full details of a specific operation, including cell values.

```
xhist show <file.xlsx> <seq>
```

**Output:**

```json
{
  "seq": 1,
  "ts": "2024-01-15T10:30:00Z",
  "action": "write",
  "sheet": "Sheet1",
  "range": "A1:C1",
  "message": "Adding headers",
  "values": [["Revenue", "Cost", "Profit"]]
}
```

---

### `xhist sheets`

List sheets in the Excel file and their used dimensions.

```
xhist sheets <file.xlsx>
```

**Output:**

```json
[
  {"name": "Sheet1", "rows": 100, "cols": 10},
  {"name": "Sheet2", "rows": 50, "cols": 5}
]
```

Does not log an operation (this is file metadata, not a cell read).

---

### `xhist state`

Reconstruct the known spreadsheet state by replaying all WRITE ops from the history.

```
xhist state <file.xlsx> [flags]
```

| Flag | Description |
|------|-------------|
| `--sheet <name>` | Show a specific sheet only |
| `--at <seq>` | Reconstruct state as of operation N |
| `--range <range>` | Show a specific range only |
| `--diff` | Compare reconstructed state vs actual Excel file |

**Output:** JSON object keyed by sheet name, values as row-major arrays.

```json
{
  "Sheet1": {
    "range": "A1:C3",
    "values": [
      ["Revenue", "Cost", "Profit"],
      [1000, 500, 500],
      [2000, 800, 1200]
    ]
  }
}
```

The `--diff` flag is especially useful for detecting out-of-band edits (someone edited the xlsx without going through xhist).

---

### `xhist info`

Show metadata and stats about the history file.

```
xhist info <file.xlsx>
```

**Output:**

```json
{
  "target": "budget.xlsx",
  "created": "2024-01-15T10:30:00Z",
  "ops": 42,
  "reads": 15,
  "writes": 27,
  "last_op": "2024-01-15T11:45:00Z",
  "sheets_touched": ["Sheet1", "Sheet2"],
  "has_footer": true,
  "index_stale": false,
  "metadata": {
    "agent.name": "excel-agent",
    "session.id": "abc-123"
  }
}
```

---

### `xhist verify`

Check integrity of the `.xhist` file. Validates every record CRC.

```
xhist verify <file.xlsx>
```

**Success:**

```json
{"ok": true, "records": 44, "ops": 42}
```

**Corruption detected:**

```json
{"ok": false, "records_valid": 30, "corruption_offset": 4892, "error": "CRC mismatch at record 31"}
```

Exit code 1 on corruption.

---

### `xhist reindex`

Rebuild the sidecar index from the log file.

```
xhist reindex <file.xlsx>
```

**Output:**

```json
{"entries": 42, "index_file": "budget.xhist.idx"}
```

---

### `xhist repair`

Truncate a corrupted `.xhist` file at the last valid record.

```
xhist repair <file.xlsx> [flags]
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Report what would be truncated without modifying the file |

**Output:**

```json
{"truncated_at": 4892, "records_kept": 30, "records_lost": 14, "bytes_removed": 2048}
```

---

## Agent Integration Patterns

### Recommended Agent Workflow

```
1. xhist sheets budget.xlsx              → discover structure
2. xhist read budget.xlsx Sheet1!A1:Z1   → read headers
3. xhist read budget.xlsx Sheet1!A1:D20  → read data
4. xhist write budget.xlsx Sheet1!E1 "Growth" -m "Adding growth rate column"
5. xhist write budget.xlsx Sheet1!E2:E20 --json '[...]' -m "Calculating growth rates"
6. xhist log budget.xlsx --last 5        → review recent operations
```

### Resuming After Context Loss

When an agent starts a new session on a file that already has history:

```
1. xhist info budget.xlsx                → understand scope
2. xhist log budget.xlsx --last 20       → recall recent operations
3. xhist log budget.xlsx --last 5 --with-values  → see actual values for recent ops
4. ... continue work ...
```

This is the core value proposition: the agent recovers its working context from the history file instead of relying on its context window.

### Real-Time Monitoring (Human)

```
$ xhist log budget.xlsx --follow --human
```

Watch the agent work in real time. Each operation appears as it's logged.
