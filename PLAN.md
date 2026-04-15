# xhist Implementation Plan

## What

A "git for Excel" CLI tool that logs every read/write operation an AI agent performs on a spreadsheet to an append-only binary file. The agent never touches the `.xlsx` directly — all operations go through `xhist`, which proxies the Excel operation and records it. This gives agents a durable history they can replay to recover context without relying on their context window.

## Specs

- [Binary format spec](docs/format.md)
- [CLI spec](docs/cli.md)

## Stack

| Dependency | Purpose | Version |
|---|---|---|
| `github.com/xuri/excelize/v2` | Excel read/write | v2.10+ |
| `github.com/urfave/cli/v3` | CLI framework | v3.8+ |
| `github.com/gofrs/flock` | Cross-platform file locking (Linux/macOS/Windows) | latest |
| `hash/crc32` (stdlib) | Record integrity — CRC-32/IEEE | — |
| `encoding/binary` (stdlib) | Binary record encoding, little-endian | — |

3 external dependencies. Everything else is stdlib.

## Architecture

```
cmd/
  main.go              CLI entrypoint, subcommand registration
  commands.go          Command definitions (init, read, write, log, show, sheets, state, info, verify, repair, reindex)
  output.go            JSON / human output helpers

pkg/
  format/
    writer.go          Append records to .xhist file (preamble, Header, Op, Metadata, Footer)
    reader.go          Sequential record reader (streaming-capable)
    record.go          Record types, opcodes, framing (encode/decode)
    cell.go            Cell value encoding/decoding (Empty, String, Number, Bool, Formula, Error)
    index.go           Sidecar .xhist.idx read/write/rebuild
    format.go          Constants (magic bytes, version, opcodes, action types)

  excel/
    excel.go           excelize wrapper — read/write cells, list sheets, get dimensions
    types.go           Cell value types bridging excelize CellType → xhist cell encoding

  lock/
    lock.go            flock wrapper — acquire/release exclusive lock on .xhist + .xlsx
```

## Build Order

Ordered by dependency. Each phase produces something testable.

### Phase 1: Binary format (pkg/format/)

The foundation. No Excel, no CLI — just the binary reader/writer with tests.

1. **Constants & types** — `format.go`, `record.go`
   - Magic bytes, version, opcodes (Header 0x01, Op 0x02, Metadata 0x03, Footer 0xFF)
   - Action types (READ=1, WRITE=2)
   - Record struct, lpstring encode/decode
   - Cell type constants

2. **Cell encoding** — `cell.go`
   - Encode/decode each cell type: Empty, String, Number, Boolean, Formula (text + cached value), Error
   - Round-trip tests for each type
   - Edge cases: empty string vs empty cell, NaN, Inf, unicode, long strings

3. **Writer** — `writer.go`
   - `NewWriter(w io.Writer)` — writes preamble (magic + version)
   - `WriteHeader(createdAt, targetFile)` — first record
   - `WriteOp(op Op)` — core operation record with CRC-32
   - `WriteMetadata(key, value)`
   - `WriteFooter(opCount, lastSeq)`
   - Internal: reusable byte buffer, CRC computation over opcode+length+payload
   - Tests: write records, verify bytes match expected layout

4. **Reader** — `reader.go`
   - `NewReader(r io.Reader)` — validates preamble
   - `Next() (Record, error)` — returns next record, verifies CRC
   - Skips unknown opcodes (forward compat)
   - Detects incomplete trailing records (returns io.EOF, not error)
   - Detects corruption (CRC mismatch → specific error with offset)
   - Tests: read back what writer wrote, corruption detection, unknown opcode skipping

5. **Index** — `index.go`
   - `BuildIndex(xhistPath) error` — scan log, write .xhist.idx
   - `ReadIndex(idxPath) ([]IndexEntry, error)`
   - `IsStale(idxPath, xhistPath) bool` — compare LogSize vs actual
   - Tests: build index, verify entries match ops, staleness detection

### Phase 2: Excel proxy (pkg/excel/)

Thin wrapper around excelize. Translates between excelize types and xhist cell types.

6. **Excel operations** — `excel.go`, `types.go`
   - `ReadCells(path, sheet, cellRange) ([][]Cell, error)` — read cells with type detection via `GetCellType()`, formula text via `GetCellFormula()`, cached value via `CalcCellValue()`
   - `WriteCells(path, sheet, cellRange, values [][]Cell) error` — write cells, handle formula strings (starting with `=`)
   - `ListSheets(path) ([]SheetInfo, error)` — sheet names + dimensions via `GetSheetDimension()`
   - `CreateWorkbook(path) error` — create empty xlsx
   - Cell type bridge: excelize `CellType` → xhist cell type byte
   - Tests with real xlsx files (create temp files, write, read back, verify types)

### Phase 3: File locking (pkg/lock/)

7. **Lock manager** — `lock.go`
   - `Acquire(xhistPath, xlsxPath) (Unlocker, error)` — exclusive flock on both files
   - `Unlocker.Release() error`
   - Lock file convention: `<file>.lock` sidecar
   - Tests: verify mutual exclusion (goroutine contention test)

### Phase 4: CLI (cmd/)

Wire everything together. Each command follows the same pattern: acquire lock → do Excel op → append record → release lock → output JSON.

8. **Output helpers** — `output.go`
   - `OutputJSON(w io.Writer, v any)` — `json.NewEncoder` with indent
   - `OutputNDJSON(w io.Writer, v any)` — single-line JSON + newline
   - `OutputHuman(w io.Writer, s string)` — plain text
   - Global `--human` flag handling via root command

9. **Core commands** — `commands.go`
   - `init` — create .xhist (+ empty xlsx if needed), write Header + optional Metadata
   - `read` — lock → read xlsx → append READ op → unlock → output JSON values
   - `write` — lock → write xlsx → append WRITE op → unlock → output confirmation
   - `sheets` — read xlsx sheet list (no op logged)
   - `log` — read .xhist, filter, output. `--follow`: poll file size, emit NDJSON
   - `show` — read single op by sequence number, output with values
   - `state` — replay all WRITE ops, reconstruct cell state, optionally diff vs actual xlsx
   - `info` — read header + scan/index for stats
   - `verify` — scan all records, check CRCs, report
   - `repair` — truncate at last valid record
   - `reindex` — rebuild sidecar index

10. **Integration tests**
    - Full workflow: init → write → read → log → show → state → verify
    - Concurrent agent simulation (two goroutines writing through CLI)
    - Corruption recovery: truncate mid-record, verify detects, repair fixes
    - Streaming: write ops in background, `--follow` picks them up

## Design Decisions (carried from spec)

- **1:1 mapping** — one `.xhist` per `.xlsx`
- **Reads log values** — agent memory of what it saw, not just what it wrote
- **Append-only** — no compaction, no rollback. AI agent scale (hundreds to low thousands of ops) doesn't need it
- **No compression** — cell data is small. Adds complexity for negligible gain
- **Auto-init** — first `read`/`write` creates `.xhist` if absent. `init` is optional
- **`--message` required on writes, optional on reads**
- **File locking via flock** — serialized access. Agent throughput is LLM-bound, not I/O-bound
- **Sidecar index is disposable** — delete and rebuild anytime
- **Manual binary encoding** — reusable buffers, `binary.LittleEndian.PutUint32()`, zero-alloc hot path. Same pattern as mcap
- **CRC-32/IEEE per record** — covers opcode + length + payload. Detects corruption at record granularity
