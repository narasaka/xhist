# Per-Workspace xhist — Implementation Plan

**Status:** Proposed
**Author:** AI-assisted
**Date:** 2026-04-17

## Problem

xhist currently creates one `.xhist` file per `.xlsx` file. In practice, a workspace can have many spreadsheets — sometimes nested in subdirectories — producing a proliferation of `.xhist` files that are hard to collect and distribute.

The core value of xhist is as a **distributable log**: copy the artifact, hand it to a review tool or pipeline, and it knows everything about the writes/edits/deletes the spreadsheets went through. Multiple scattered files defeat that purpose.

## Goal

**One `.xhist` file per workspace.** Copy it, ship it, review it. The file is fully self-contained — a consumer needs nothing else to understand every operation across every spreadsheet.

## Design Decisions

### 1. File Naming

**Convention:** `{directory-name}.xhist` in the workspace root.

```
~/work/q1-budget/
  budget.xlsx
  revenue.xlsx
  reports/forecast.xlsx
  q1-budget.xhist          <-- named after the directory
  q1-budget.xhist.idx      <-- sidecar index (not distributed)
```

`xhist init` creates `{cwd-basename}.xhist` by default.
`xhist init <name>` creates `<name>.xhist` (user-chosen name).
`--force` overwrites if one already exists.

**Discovery:** Commands auto-discover the workspace log by searching for a single `*.xhist` file in the current directory, then walking up parent directories. `--workspace <path>` overrides discovery.

Error if zero or multiple `*.xhist` files are found without `--workspace`.

### 2. Binary Format — Version 2

Magic stays `XHIST\0`. Version byte changes from `0x01` to `0x02`.

#### Header Record (`0x01`) — Changed

The Header becomes workspace-scoped. `TargetFile` is removed.

| Field         | Type     | Description                              |
|---------------|----------|------------------------------------------|
| CreatedAt     | int64    | Workspace log creation time (Unix ms)    |
| WorkspaceName | lpstring | Human-readable workspace name            |

One Header, first record in the file, same as v1.

#### Op Record (`0x02`) — Changed

`TargetFile` added as the first field. Every Op is self-describing — a consumer can parse any record in isolation without cross-referencing the Header or a file table.

| Field      | Type     | Description                                      |
|------------|----------|--------------------------------------------------|
| TargetFile | lpstring | Workspace-relative path (e.g. `budget.xlsx`, `reports/forecast.xlsx`) |
| Timestamp  | int64    | Operation time (Unix ms)                         |
| Sequence   | uint32   | Global monotonic counter, 1-based                |
| Action     | uint8    | `1` = READ, `2` = WRITE                          |
| Sheet      | lpstring | Sheet name                                       |
| Range      | lpstring | Cell range                                       |
| Message    | lpstring | Agent's explanation                              |
| NumRows    | uint32   | Row count                                        |
| NumCols    | uint32   | Column count                                     |
| Cells      | Cell[]   | `NumRows * NumCols` cells, row-major             |
| HasComments| uint8    | `0` = no comments, `1` = comments follow         |
| NumComments| uint32   | (only if HasComments=1) Comment count             |
| Comments   | CommentEntry[] | (only if HasComments=1)                     |

#### CommentOp Record (`0x04`) — Changed

Same change: `TargetFile` added as first field.

| Field      | Type     | Description                                      |
|------------|----------|--------------------------------------------------|
| TargetFile | lpstring | Workspace-relative path                          |
| Timestamp  | int64    | Operation time (Unix ms)                         |
| Sequence   | uint32   | Global monotonic counter                         |
| Action     | uint8    | `1` = SET, `2` = GET, `3` = DELETE               |
| Sheet      | lpstring | Sheet name                                       |
| Range      | lpstring | Cell range                                       |
| Message    | lpstring | Agent's explanation                              |
| NumEntries | uint32   | Comment entry count                              |
| Entries    | CommentEntry[] | Comment entries                             |

#### Metadata (`0x03`) — Unchanged

Same key-value pairs. New reserved keys:

| Key               | Description                    |
|-------------------|--------------------------------|
| `workspace.name`  | Workspace display name         |
| `workspace.root`  | Original absolute path (informational, not used for resolution) |

#### Footer (`0xFF`) — Unchanged

| Field        | Type   | Description                    |
|--------------|--------|--------------------------------|
| OpCount      | uint32 | Total Op+CommentOp records     |
| LastSequence | uint32 | Last assigned sequence number  |

#### What Stays the Same

- Record framing: opcode + length + payload + CRC (9 bytes overhead)
- Cell encoding (all 6 types)
- Metadata records
- Footer record
- Append-only semantics
- CRC-32/IEEE per record

### 3. Sequence Numbers — Global

One monotonic counter across all files in the workspace. Op 17 happened before Op 18, regardless of which spreadsheet they target.

Per-file sequence is not stored. If needed (e.g. `xhist log budget.xlsx`), it can be derived at read time by filtering and re-numbering.

### 4. Target File Paths — Relative and Normalized

All `TargetFile` values are:
- **Relative** to the workspace root (the directory containing the `.xhist` file)
- **Forward-slash separated** (even on Windows)
- **No leading `./`**

Examples: `budget.xlsx`, `reports/forecast.xlsx`

This is critical for portability. The file must make sense on any machine.

### 5. Sidecar Index — Extended

Index entry gains a `TargetFile` field:

| Field      | Type     | Description                                |
|------------|----------|--------------------------------------------|
| Offset     | uint64   | Byte offset of the record in `.xhist`      |
| Opcode     | uint8    | Record type                                |
| Timestamp  | int64    | Copy of Op/CommentOp timestamp             |
| Sequence   | uint32   | Copy of Op/CommentOp sequence              |
| Action     | uint8    | Copy of Op/CommentOp action                |
| FileLen    | uint16   | Byte length of target file path            |
| File       | bytes    | Target file path, UTF-8                    |
| SheetLen   | uint16   | Byte length of sheet name                  |
| Sheet      | bytes    | Sheet name, UTF-8                          |

Index version bumps from `0x02` to `0x03`.

The index is NOT distributed — it's a local acceleration structure.

### 6. Locking

- **Workspace log lock:** `{name}.xhist.lock` — acquired for all log appends
- **Excel file lock:** `{file}.xlsx.lock` — acquired for spreadsheet mutations (same as today)

Both locks acquired for write operations. Only workspace lock needed for log-only operations (e.g. `xhist log`).

Expected usage: single agent per workspace. Coarse workspace lock is acceptable.

### 7. Version-Aware Reader

The reader checks byte 7 (version):
- `0x01`: v1 format — single-file log, Op records lack `TargetFile`, Header has `TargetFile`
- `0x02`: v2 format — workspace log, Op records have `TargetFile`, Header has `WorkspaceName`

v1 files remain readable. The reader infers `TargetFile` for all ops from the v1 Header.

## CLI Changes

### `xhist init`

**Before:**
```
xhist init <file.xlsx> [--agent NAME] [--model ID] [--session ID] [--force]
```

**After:**
```
xhist init [name] [--agent NAME] [--model ID] [--session ID] [--force]
```

- `xhist init` — creates `{cwd-basename}.xhist` in the current directory
- `xhist init my-project` — creates `my-project.xhist` in the current directory
- `--force` overwrites existing
- Writes Header with `WorkspaceName` = the chosen name
- No longer requires or references a specific `.xlsx` file

### `xhist write`

**Before:**
```
xhist write <file.xlsx> <range> [value] -m "message" [flags]
```

**After:** Same syntax. Internally:
1. Resolve `<file.xlsx>` to a workspace-relative path
2. Auto-discover workspace `.xhist` (or use `--workspace`)
3. Auto-init workspace `.xhist` if none exists (using cwd basename)
4. Append Op with `TargetFile` set to the relative path

### `xhist read`

Same syntax as `write`. Auto-discovers workspace log.

### `xhist log`

**Before:**
```
xhist log <file.xlsx> [flags]
```

**After:**
```
xhist log [file.xlsx] [flags]
```

- `xhist log` — show all operations across all files
- `xhist log budget.xlsx` — filter to one file
- All existing flags (`--sheet`, `--action`, `--since`, `--last`, `--with-values`, `--follow`) still work
- New: `--file <path>` flag as alternative to positional arg for filtering

Output gains a `"file"` field:
```json
{"seq": 1, "file": "budget.xlsx", "ts": "...", "action": "write", "sheet": "Sheet1", "range": "A1", "message": "..."}
```

### `xhist show`

**Before:**
```
xhist show <file.xlsx> <seq>
```

**After:**
```
xhist show <seq>
```

Sequence is globally unique — no need to specify file. Output includes `"file"` field.

### `xhist state`

**Before:**
```
xhist state <file.xlsx> [flags]
```

**After:**
```
xhist state <file.xlsx> [flags]
```

Unchanged syntax — state reconstruction requires a target file. Filters ops by `TargetFile` internally.

### `xhist info`

**Before:**
```
xhist info <file.xlsx>
```

**After:**
```
xhist info [file.xlsx]
```

- `xhist info` — workspace-level stats (all files, total ops, file list)
- `xhist info budget.xlsx` — file-specific stats (filtered)

Output gains `"files"` array:
```json
{
  "workspace": "q1-budget",
  "created": "2024-01-15T10:30:00Z",
  "files": ["budget.xlsx", "revenue.xlsx", "reports/forecast.xlsx"],
  "ops": 142,
  "reads": 50,
  "writes": 92,
  ...
}
```

### `xhist sheets`

**Before:**
```
xhist sheets <file.xlsx>
```

**After:** Unchanged. This reads the xlsx directly, not the log.

### `xhist verify`, `xhist repair`, `xhist reindex`

**Before:**
```
xhist verify <file.xlsx>
xhist repair <file.xlsx>
xhist reindex <file.xlsx>
```

**After:** No positional arg needed — operates on the discovered workspace `.xhist`.
```
xhist verify [--workspace <path>]
xhist repair [--workspace <path>] [--dry-run]
xhist reindex [--workspace <path>]
```

### `xhist comment`

Same syntax as before, just auto-discovers workspace log like `read`/`write`.

### `xhist migrate` — New Command

```
xhist migrate [--name NAME] [--dry-run]
```

Scans current directory (recursively) for v1 `.xhist` files, merges them into a single v2 workspace log.

- Merge order: by timestamp. Ties broken by filepath, then old per-file sequence.
- Global sequence assigned in merge order (1, 2, 3, ...).
- Old `.xhist` files are **not deleted** — user removes them manually after verifying.
- `--dry-run` shows what would be merged without writing.
- `--name` sets the workspace name (default: cwd basename).

### Global Flag

All commands gain:
```
--workspace <path>    Path to workspace .xhist file (overrides auto-discovery)
```

## Implementation Order

### Phase 1: Format Layer (`internal/format/`)

1. Add `FormatVersion` field to Reader; detect v1 vs v2 from preamble
2. Add `TargetFile` to `Op` and `CommentOp` structs
3. Implement v2 encode/decode for Op, CommentOp, Header
4. v1 decode remains unchanged; v1 reader populates `TargetFile` from Header
5. Writer always writes v2
6. Update index entry struct and encode/decode with `TargetFile`
7. Bump index version to `0x03`
8. Tests for v2 round-trip, v1 backward compat

### Phase 2: Workspace Discovery (`internal/workspace/` — new package)

1. `Discover(startDir) (string, error)` — find `*.xhist` in dir, walk up
2. `RelativePath(workspaceRoot, xlsxPath) string` — normalize to forward-slash relative path
3. `DefaultName(dir) string` — basename of directory
4. Tests

### Phase 3: CLI Refactor (`internal/cli/`)

1. Replace `xhistPath()` with workspace discovery
2. Replace `ensureInit()` with workspace-aware auto-init
3. Update `init` command — new signature, workspace creation
4. Update `write`, `read`, `comment` — resolve target file, use workspace log
5. Update `log` — optional file arg, add `"file"` to output
6. Update `show` — seq-only lookup, add `"file"` to output
7. Update `state` — filter by target file
8. Update `info` — workspace-level and file-level modes
9. Update `verify`, `repair`, `reindex` — workspace-scoped
10. Update locking — workspace log lock + xlsx lock
11. Update `lastSequence()` — scan workspace log (global sequence)

### Phase 4: Migration (`internal/cli/migrate.go` — new)

1. Scan for v1 `.xhist` files recursively
2. Read all ops from each, tag with source file
3. Sort by timestamp (stable sort with tiebreakers)
4. Write v2 workspace log with global sequence
5. `--dry-run` mode
6. Tests

### Phase 5: Documentation & Cleanup

1. Update `docs/format.md` — v2 spec
2. Update `docs/cli.md` — new command signatures
3. Update `README.md`
4. Update `skills/xhist/SKILL.md` — agent instructions
5. Remove this `PLAN.md` or move to `docs/`

## What We're NOT Doing

- **File table / fileId optimization** — repeated path strings are fine for now. Cell payloads dominate binary size. Optimize later if logs hit millions of tiny ops.
- **Multi-agent concurrency** — single agent per workspace. No sharded logs or WAL.
- **Compression** — cell data is small. Not worth the complexity.
- **Breaking v1 support** — v1 files remain readable.
