---
name: xhist
description: "Use xhist CLI to read, write, and inspect Excel files with full operation history. MUST USE whenever the user asks to work with .xlsx spreadsheets, read or write cells, inspect spreadsheet data, or track changes to Excel files. Also use when the user mentions xhist, workspace history, cell ranges, or wants to review what was previously done in a project. This skill ensures all Excel operations are logged to a workspace-level history file and are recoverable."
---

# xhist, Excel Operation Logger

xhist is a CLI tool that proxies all Excel read/write operations through an append-only binary log. The agent never opens `.xlsx` files directly — every operation goes through xhist, which records it to a workspace-level history file.

## Why This Matters

xhist is both an interface and a memory layer:

- **Interface**: every read, write, comment, and confusion/resolution the agent performs on a workbook goes through `xhist`. There is no other durable artifact for spreadsheet work.
- **Memory / diary**: every operation is logged with a `-m "<reason>"` message, a timestamp, the cell values, the sheet/range, and the target file. When context is lost, replay the log with `xhist log` to see exactly what was done and why — concrete evidence, not guesses.

Write messages like you would write git commit messages: explain *why* the write is happening and what the source was. The log is the only persistent memory that survives across agent turns and sessions.

## Core Concepts

- One `.xhist` file tracks **all** `.xlsx` files in a workspace
- Workspace discovery: xhist automatically walks up from the current directory to find the nearest `.xhist` file; if none is found, read/write/comment/confuse auto-create one
- Use `--workspace <path>` to override discovery and point at a specific `.xhist` file
- All output is JSON to stdout by default; `-H`/`--human` switches to a readable table for logs
- Errors are emitted as `{"error": "message"}` on stderr with a non-zero exit code
- Shell quoting: ranges with `!` must be single-quoted (e.g. `'Sheet1!A1'`)

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
| `42`, `3.14`, `-456` | Number |
| `true`, `false` | Boolean |
| `null` | Empty cell |
| `"=SUM(A1:A10)"` | Formula (string starting with `=`) |

Ranges are row-major arrays of arrays:
```json
[["Name", "Q1", "Q2"], ["Revenue", 1000, 2000]]
```

Reads always return the **raw stored value** — numbers come back as JSON numbers, not as display-formatted strings. A cell that renders as `"231,521"` or `"(456)"` in Excel is returned as `231521` and `-456` respectively.

## Commands

### Initialize workspace (optional)

```bash
xhist init [name] --agent "my-agent" --model "gpt-4" --session "abc-123"
```

Creates `{name}.xhist` in the current directory. Not required; `read`, `write`, and `comment` auto-create a workspace log if none is found.

### Discover structure

```bash
xhist sheets budget.xlsx
```

Returns sheet names with row/column counts. Does not log an operation.

### Read cells

```bash
xhist read budget.xlsx 'Sheet1!A1'                                   # single cell → scalar
xhist read budget.xlsx 'Sheet1!A1:C10' -m "Reading quarterly data"   # range → 2D array
xhist read budget.xlsx 'Sheet1!A1:C10' --no-log                      # skip logging
```

The `-m` flag is optional on reads. Use it to leave a trail of reasoning.

### Write cells

```bash
# Single positive value
xhist write budget.xlsx 'Sheet1!A1' "Revenue" -m "Adding header"
xhist write budget.xlsx 'Sheet1!B1' 1000 -m "Adding revenue value"

# Negative numbers: put flags first, then `--`, then the value, so the shell
# does not interpret `-1234` as a flag.
xhist write budget.xlsx 'Sheet1!B2' -m "Net loss" -- -1234

# Alternative for negative numbers or any literal: use --json with a scalar
xhist write budget.xlsx 'Sheet1!B2' --json '-1234' -m "Net loss"

# Range from inline JSON
xhist write budget.xlsx 'Sheet1!A1:B2' --json '[["Name","Value"],["Revenue",1000]]' -m "Adding data"

# Range from file or stdin
xhist write budget.xlsx 'Sheet1!A1:B2' -f data.json -m "From file"
echo '[["A","B"]]' | xhist write budget.xlsx 'Sheet1!A1:B1' --stdin -m "From stdin"
```

The `-m` flag is **required** on writes. Always explain why the write is being performed and cite the source.

Output on success: `{"seq": <N>, "cells_written": <M>}`.

### Manage cell comments

Comments are native Excel notes; they persist in the workbook and are also recorded in the operation log. Use them for citations and provenance.

```bash
# Set a comment on a single cell (requires -m)
xhist comment set budget.xlsx 'Sheet1!A1' "Source: SAP FY25-Q1" \
  -m "Citation for Q1 revenue" --author "agent-1"

# Set comments on a range via JSON grid (empty string = no comment)
xhist comment set budget.xlsx 'Sheet1!A1:B2' \
  --json '[["Source: SAP",""],["Source: Oracle","Source: Bloomberg"]]' \
  -m "Adding citations" --author "agent-1"

# Get a single cell's comment (no -m required)
xhist comment get budget.xlsx 'Sheet1!A1'
# → {"cell": "A1", "author": "agent-1", "text": "Source: SAP FY25-Q1"}

# Get all comments in a range (sparse — only cells with comments appear)
xhist comment get budget.xlsx 'Sheet1!A1:C10'

# Delete a cell's comment (requires -m)
xhist comment delete budget.xlsx 'Sheet1!A1' -m "Removing stale citation"

# Delete all comments in a range
xhist comment delete budget.xlsx 'Sheet1!A1:C10' -m "Clearing all citations"
```

`xhist comment --help` lists the three subcommands if you ever forget.

### Record confusions

Use `xhist confuse` when you cannot confidently write a value yet. Confusions are stored in the same `.xhist` artifact as reads, writes, and comments.

```bash
# Raise a cell-level confusion
xhist confuse raise budget.xlsx 'Sheet1!B2' \
  --archetype GAP \
  --headline "Missing revenue value" \
  --description "No source value was found for the requested period" \
  --payload '{"sourceField":"Revenue"}' \
  -m "Needs reconciliation before writing"

# Resolve it later
xhist confuse resolve <id> \
  --value 123 \
  --confidence high \
  --reasoning "Source row confirms 123" \
  --source auto \
  -m "Resolved from source workbook"

# Or skip it with a reason
xhist confuse skip <id> -r "Out of scope for this deliverable"
```

Supported action filters:

```bash
xhist log --action confusion          # all confusion ops
xhist log --action confusion_raise    # only raised confusions
xhist log --action confusion_resolve  # only resolutions
xhist log --action confusion_skip     # only skipped confusions
```

### Review history

```bash
xhist log                                       # full workspace log
xhist log budget.xlsx                           # filter by target file
xhist log --action write                        # only writes
xhist log --action read                         # only reads
xhist log --action comment                      # all comment ops (set + get + delete)
xhist log --action comment_set                  # only comment set ops
xhist log --action confusion                    # all confusion ops (raise + resolve + skip)
xhist log --last 10 --with-values               # last 10 ops, include cell values
xhist log --since 2024-01-01T00:00:00Z          # ops after a timestamp
xhist log --sheet Sheet1                        # only ops on Sheet1
xhist log --human                               # readable table instead of JSON
xhist log --follow                              # stream new ops (NDJSON)
```

`--action comment` matches all three comment subtypes (`comment_set`, `comment_get`, `comment_delete`) as a shorthand. `--action confusion` matches all three confusion subtypes (`confusion_raise`, `confusion_resolve`, `confusion_skip`).

### Show a specific operation

```bash
xhist show 42
```

Returns full details for operation sequence number 42, including cell values, comment entries, or confusion payload/resolution details.

### Recover context (new session)

Treat the log as your diary. Before writing anything new, read what was already done:

```bash
xhist info                                        # workspace overview, file list
xhist log --last 20 --with-values                 # recent timeline with values
xhist log budget.xlsx --last 5 --with-values      # focus on one file
xhist log --action comment --with-values          # recall all citations
xhist log --action confusion                      # recall unresolved/reconciled confusions
xhist state budget.xlsx                           # full reconstructed state
```

### Reconstruct state

```bash
xhist state budget.xlsx                    # reconstruct sheet grid from WRITE ops
xhist state budget.xlsx --at 10            # state as of operation 10
xhist state budget.xlsx --range Sheet1!A1:C10
xhist state budget.xlsx --sheet Sheet1
xhist state budget.xlsx --diff             # compare vs actual xlsx (detect out-of-band edits)
xhist state budget.xlsx --with-comments    # include comment state per sheet
```

With `--with-comments`, each sheet object has a `comments` array of `{cell, author, text}` entries in addition to `range` and `values`.

### Maintenance

```bash
xhist verify                          # check workspace log integrity
xhist reindex                         # rebuild sidecar index
xhist repair                          # truncate corrupted log at last valid record
xhist migrate                         # import legacy v1 .xhist files into a v2 workspace
```

### Global flags

```bash
xhist --workspace /path/to/ws.xhist ...   # override workspace discovery
xhist --human ...                          # human-readable output where supported
xhist --version                            # show version
```

## Recommended Workflow

### First time in a workspace

```bash
xhist sheets budget.xlsx                                    # discover structure
xhist read budget.xlsx 'Sheet1!A1:Z1' -m "Read headers"     # understand layout
xhist read budget.xlsx 'Sheet1!A1:D20' -m "Read data"       # sample values
# analyze...
xhist write budget.xlsx 'Sheet1!E1' "Growth" -m "Adding growth column from 2024 10-K p.47"
xhist comment set budget.xlsx 'Sheet1!E1' "Source: 2024 10-K p.47" -m "Citation" --author "agent-1"
```

### Resuming after context loss

```bash
xhist info                                          # what files are tracked?
xhist log --last 20 --with-values                   # what happened recently?
xhist log --action comment --with-values            # what citations exist?
xhist state budget.xlsx --with-comments             # current known grid + comments
# continue work from here
```

## Common Mistakes

- Forgetting `-m` on writes, `comment set`, or `comment delete` (it is required, commands fail without it)
- Using bare `xhist comment ...` — you must use the subcommand: `xhist comment set|get|delete`
- Using `--action comment_write` or `--action comment_setting` etc. — the valid action filters are `read`, `write`, `comment` (shorthand), `comment_set`, `comment_get`, `comment_delete`
- Writing a negative number without a `--` terminator or `--json`: `xhist write f.xlsx A1 -456 -m "..."` will be parsed as `-4` flag; use `xhist write f.xlsx A1 -m "..." -- -456` or `--json '-456'`
- Not quoting `!` in ranges — always use `'Sheet1!A1'`
- Passing `.xhist` instead of `.xlsx` as the file argument for read/write
- Writing formulas without the `=` prefix (they will be stored as strings)
- Expecting `xhist read` to return formatted strings — it returns raw values; compare raw against raw
