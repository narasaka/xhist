---
name: xhist
description: "Use xhist CLI to read, write, and inspect Excel files with full operation history. MUST USE whenever the user asks to work with .xlsx spreadsheets, read or write cells, inspect spreadsheet data, or track changes to Excel files. Also use when the user mentions xhist, spreadsheet history, cell ranges, or wants to review what was previously done to a workbook. This skill ensures all Excel operations are logged and recoverable."
---

# xhist — Excel Operation Logger

xhist is a CLI tool that proxies all Excel read/write operations through an append-only binary log. The agent never opens .xlsx files directly. Every operation goes through xhist, which records it for later replay.

## Why This Matters

AI agents lose context between sessions. xhist solves this: every read and write is logged with timestamps, messages, and cell values. When starting a new session on a file with history, replay the log to recover full working context.

## Core Concepts

- One `.xhist` file tracks exactly one `.xlsx` file
- `budget.xlsx` has its history in `budget.xhist` (same directory)
- First operation auto-creates both files if needed
- All output is JSON to stdout by default
- Errors go to stderr as `{"error": "message"}` with exit code 1
- Shell quoting: ranges with `!` must be single-quoted in interactive bash and zsh (e.g. `'Sheet1!A1'`)

## Cell Addressing

Standard Excel notation. Sheet name is delimited by `!`.

| Example | Meaning |
|---------|---------|
| `'Sheet1!A1'` | Single cell on Sheet1 |
| `'Sheet1!A1:C10'` | Rectangular range on Sheet1 |
| `A1` | Single cell on the first sheet |
| `A1:C10` | Range on the first sheet |

Always single-quote ranges containing `!` to prevent shell history expansion.

## Cell Values in JSON

| JSON | Excel Type |
|------|------------|
| `"text"` | String |
| `42`, `3.14` | Number |
| `true`, `false` | Boolean |
| `null` | Empty cell |
| `"=SUM(A1:A10)"` | Formula (string starting with `=`) |

Ranges are row-major arrays of arrays:
```json
[["Name", "Q1", "Q2"], ["Revenue", 1000, 2000]]
```

## Commands

### Discover structure

```bash
xhist sheets budget.xlsx
```

Returns sheet names with row/column counts. Does not log an operation.

### Read cells

```bash
# Single cell (returns scalar)
xhist read budget.xlsx 'Sheet1!A1'

# Range (returns 2D array)
xhist read budget.xlsx 'Sheet1!A1:C10' -m "Reading quarterly data"

# Read without logging (for internal checks)
xhist read budget.xlsx 'Sheet1!A1:C10' --no-log
```

The `-m` flag is optional on reads but recommended for context.

### Write cells

```bash
# Single cell
xhist write budget.xlsx 'Sheet1!A1' "Revenue" -m "Adding header"

# Range from JSON
xhist write budget.xlsx 'Sheet1!A1:B2' --json '[["Name","Value"],["Revenue",1000]]' -m "Adding data"

# Range from file
xhist write budget.xlsx 'Sheet1!A1:B2' -f data.json -m "Importing data"

# Range from stdin
echo '[["A","B"]]' | xhist write budget.xlsx 'Sheet1!A1:B1' --stdin -m "Piped data"
```

The `-m` flag is required on writes. Always explain why the write is being performed.

Output on success: `{"seq": 1, "cells_written": 2}`

### Review history

```bash
# Full log
xhist log budget.xlsx

# Last 5 operations
xhist log budget.xlsx --last 5

# Filter by action
xhist log budget.xlsx --action write

# Include cell values
xhist log budget.xlsx --last 3 --with-values

# Human-readable table
xhist log budget.xlsx --human

# Stream new operations in real time (NDJSON)
xhist log budget.xlsx --follow
```

### Show a specific operation

```bash
xhist show budget.xlsx 3
```

Returns full details including cell values for operation with sequence number 3.

### Recover context (new session)

When resuming work on a file:

```bash
xhist info budget.xlsx                        # scope and stats
xhist log budget.xlsx --last 20               # recent operations
xhist log budget.xlsx --last 5 --with-values  # recent values
```

### Reconstruct state

```bash
# Replay all writes to reconstruct known state
xhist state budget.xlsx

# State as of operation 10
xhist state budget.xlsx --at 10

# Compare reconstructed state vs actual file (detect out-of-band edits)
xhist state budget.xlsx --diff
```

### Initialize explicitly

```bash
xhist init budget.xlsx --agent "my-agent" --model "gpt-4" --session "abc-123"
```

Optional. Other commands auto-initialize. Use when you want to record agent metadata upfront.

### Integrity and maintenance

```bash
# Check file integrity
xhist verify budget.xlsx

# Repair corrupted file (truncate at last valid record)
xhist repair budget.xlsx --dry-run
xhist repair budget.xlsx

# Rebuild sidecar index
xhist reindex budget.xlsx
```

## Recommended Workflow

### First time with a file

```bash
xhist sheets budget.xlsx                           # discover structure
xhist read budget.xlsx 'Sheet1!A1:Z1' -m "Headers" # read headers
xhist read budget.xlsx 'Sheet1!A1:D20' -m "Data"   # read data
# ... analyze and write changes ...
xhist write budget.xlsx 'Sheet1!E1' "Growth" -m "Adding growth rate column"
xhist write budget.xlsx 'Sheet1!E2:E20' --json '[...]' -m "Calculating growth rates"
```

### Resuming after context loss

```bash
xhist info budget.xlsx                             # understand scope
xhist log budget.xlsx --last 20                    # recall recent ops
xhist log budget.xlsx --last 5 --with-values       # see actual values
# ... continue work ...
```

## Common Mistakes

- Forgetting `-m` on writes (it is required, the command will fail)
- Not quoting `!` in ranges (`Sheet1!A1` triggers history expansion in interactive bash and zsh, use `'Sheet1!A1'`)
- Passing `.xhist` instead of `.xlsx` as the file argument
- Writing formulas without the `=` prefix (they will be stored as strings)
