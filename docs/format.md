# xhist Binary Format Specification

**Version:** 1
**Status:** Draft

## Overview

xhist is an append-only binary log format that records every read and write operation an AI agent performs on an Excel file. One `.xhist` file tracks exactly one `.xlsx` file. The format is designed for:

- **Append-only writes** — O(1) per operation, no rewrites
- **Streaming reads** — a reader can tail the file for real-time operation visibility
- **Forward compatibility** — unknown record types can be skipped without parsing
- **Crash safety** — per-record CRCs detect corruption; partial trailing records are ignored

An optional sidecar index file (`.xhist.idx`) accelerates filtered queries but is always regenerable from the log.

## Notation

All multi-byte integers are **little-endian**. Signed integers use two's complement.

| Type     | Size    | Description                                     |
|----------|---------|-------------------------------------------------|
| uint8    | 1 byte  | Unsigned integer                                |
| uint16   | 2 bytes | Unsigned integer, little-endian                 |
| uint32   | 4 bytes | Unsigned integer, little-endian                 |
| uint64   | 8 bytes | Unsigned integer, little-endian                 |
| int64    | 8 bytes | Signed integer, little-endian                   |
| float64  | 8 bytes | IEEE 754 double-precision, little-endian        |
| lpstring | 4+N     | uint32 byte-length, followed by N UTF-8 bytes   |

## File Layout

```
[Preamble (7 bytes)]
[Record]*
```

### Preamble

| Offset | Size | Field   | Value                                                  |
|--------|------|---------|--------------------------------------------------------|
| 0      | 6    | Magic   | `0x58 0x48 0x49 0x53 0x54 0x00` (ASCII `XHIST` + NUL) |
| 6      | 1    | Version | `0x01` for this specification                          |

The first record after the preamble MUST be a Header record (opcode `0x01`).

## Record Framing

Every record uses the same envelope:

```
+--------+--------+---------+-------+
| Opcode | Length | Payload |  CRC  |
| 1 byte | 4 bytes| N bytes | 4 bytes|
+--------+--------+---------+-------+
```

| Field   | Type   | Description                                               |
|---------|--------|-----------------------------------------------------------|
| Opcode  | uint8  | Record type identifier                                    |
| Length  | uint32 | Byte length of Payload (0 is valid)                       |
| Payload | bytes  | Record-type-specific data, exactly `Length` bytes          |
| CRC     | uint32 | CRC-32/IEEE over the concatenation of Opcode + Length + Payload |

**Total framing overhead: 9 bytes per record.**

To skip an unknown opcode: read Length, skip `Length + 4` bytes (payload + CRC).

## Record Types

### Header (`0x01`)

MUST be the first record in the file. Exactly one per file.

| Field      | Type     | Description                              |
|------------|----------|------------------------------------------|
| CreatedAt  | int64    | File creation time (Unix milliseconds)   |
| TargetFile | lpstring | Relative path to the tracked `.xlsx` file |

### Op (`0x02`)

The core operation record. Every agent read or write produces one Op.

| Field     | Type     | Description                                   |
|-----------|----------|-----------------------------------------------|
| Timestamp | int64    | Operation time (Unix milliseconds)            |
| Sequence  | uint32   | Monotonically increasing counter, 1-based     |
| Action    | uint8    | `1` = READ, `2` = WRITE                       |
| Sheet     | lpstring | Sheet name                                    |
| Range     | lpstring | Cell range (e.g. `A1`, `A1:C10`)              |
| Message   | lpstring | Agent's explanation (empty string if not provided) |
| NumRows   | uint32   | Row count of the data grid                    |
| NumCols   | uint32   | Column count of the data grid                 |
| Cells     | Cell[]   | `NumRows * NumCols` cells in **row-major** order |

The Sequence counter resets only with a new file. It never wraps or reuses values within a file.

Action `0` is reserved. Values `3–255` are reserved for future operations.

### Metadata (`0x03`)

Arbitrary key-value pair for extensibility.

| Field | Type     | Description    |
|-------|----------|----------------|
| Key   | lpstring | Metadata key   |
| Value | lpstring | Metadata value |

Reserved key prefixes and their meanings:

| Key              | Description                    |
|------------------|--------------------------------|
| `agent.name`     | AI agent name                  |
| `agent.model`    | Model identifier               |
| `session.id`     | Groups ops into logical sessions |
| `session.start`  | Marks session beginning        |
| `session.end`    | Marks session end              |

Implementations MUST ignore unknown keys. Keys starting with `x.` are reserved for user-defined metadata.

### Footer (`0xFF`)

Optional. Written on clean close. Its absence indicates the writer did not shut down cleanly — the file is still valid up to the last complete record.

| Field        | Type   | Description                    |
|--------------|--------|--------------------------------|
| OpCount      | uint32 | Total Op records in the file   |
| LastSequence | uint32 | Last assigned sequence number  |

## Cell Encoding

Each cell is encoded as a type tag followed by type-specific data:

```
+------+-------+
| Type | Value |
| 1 byte| var  |
+------+-------+
```

| Type Byte | Name    | Value Encoding                                             |
|-----------|---------|------------------------------------------------------------|
| `0x00`    | Empty   | No value bytes (0 additional bytes)                        |
| `0x01`    | String  | lpstring                                                   |
| `0x02`    | Number  | float64                                                    |
| `0x03`    | Boolean | uint8 (`0` = false, `1` = true)                            |
| `0x04`    | Formula | lpstring (formula text) + Cell (cached computed value)     |
| `0x05`    | Error   | lpstring (e.g. `#N/A`, `#REF!`, `#VALUE!`)                 |

**Formula cells** store both the formula text and the last computed value. The cached value is encoded as a nested Cell — any type except Formula (type `0x04`). This lets a reader surface the evaluated result without needing an Excel engine.

Type bytes `0x06–0xFF` are reserved for future cell types.

## Sidecar Index

**Filename:** `<name>.xhist.idx` alongside `<name>.xhist`

The index is a disposable acceleration structure. Deleting it is always safe — rebuild with a single scan of the log.

### Index Layout

```
[Preamble (15 bytes)]
[Entry]*
```

#### Index Preamble

| Offset | Size | Field      | Description                                        |
|--------|------|------------|----------------------------------------------------|
| 0      | 6    | Magic      | `0x58 0x48 0x49 0x44 0x58 0x00` (`XHIDX` + NUL)  |
| 6      | 1    | Version    | `0x01`                                             |
| 7      | 8    | LogSize    | uint64 — `.xhist` file size when index was built   |
| 15     | 4    | EntryCount | uint32 — number of index entries                   |

#### Index Entry

One entry per Op record in the log.

| Field     | Type     | Description                                |
|-----------|----------|--------------------------------------------|
| Offset    | uint64   | Byte offset of the Op record in `.xhist`   |
| Timestamp | int64    | Copy of `Op.Timestamp`                     |
| Sequence  | uint32   | Copy of `Op.Sequence`                      |
| Action    | uint8    | Copy of `Op.Action`                        |
| SheetLen  | uint16   | Byte length of sheet name                  |
| Sheet     | bytes    | Sheet name, UTF-8, `SheetLen` bytes        |

**Staleness check:** On open, compare actual `.xhist` file size to stored `LogSize`. If they differ, the index is stale and must be rebuilt. If the log is larger, an incremental update (appending new entries) is valid.

## Streaming Protocol

### Writer

1. Open `.xhist` for append (create if absent, writing preamble + Header).
2. Encode the record (opcode + length + payload + CRC).
3. Write the complete record in a single `write(2)` call.
4. `fsync` after each record (or batch for throughput at the cost of latency).

Using a single write call ensures that a reader never sees a partial record header with a dangling length field — they either see the full record or nothing.

### Reader (Streaming)

1. Open `.xhist` for reading.
2. Validate preamble (magic + version).
3. Read records sequentially. For each: read opcode + length, read `Length` payload bytes, read CRC, verify CRC.
4. On EOF with an incomplete record (fewer bytes than expected): treat as not-yet-written. Remember the file offset of this incomplete record.
5. Poll for growth: periodically check file size (or use `inotify`/`kqueue`/`ReadDirectoryChangesW`). On growth, resume reading from the saved offset.
6. Emit each newly completed, CRC-valid record as a stream event.

A record is **complete** when `1 + 4 + Length + 4` bytes are available from the record start AND the CRC matches.

## Error Recovery

**Corruption** = CRC mismatch on an otherwise complete record.

1. All records before the corruption point are valid and usable.
2. Report the byte offset of the corrupted record.
3. **Recommended action:** Truncate the file at the last valid record boundary. All data after the corruption point is lost.
4. v1 does not attempt to scan past corruption to find subsequent valid records. This avoids false positives from payload bytes that happen to look like valid record headers.

**Incomplete trailing record** (not enough bytes for a full record) is NOT corruption — it indicates an interrupted write. Truncate to the end of the last complete record.

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Append-only, no compaction | Simplicity. For AI agent workloads (hundreds to low thousands of ops), file size is not a concern. |
| CRC-32 per record, not per chunk | Each record is independently verifiable. No need to buffer chunks. |
| Sidecar index instead of inline summary | True append-only — the log file is never rewritten. Index is a disposable cache. |
| uint32 record length (max 4 GiB/record) | Generous ceiling. A single op record is typically under 1 KiB. |
| Sequence numbers on ops | Stable references for `xhist show <seq>`. Monotonic, never reused within a file. |
| Formula cells store cached value | Agent can see computed results without an Excel engine. |
| 1:1 file mapping | One `.xhist` tracks one `.xlsx`. Simplifies addressing and portability. |
| No compression in v1 | Cell data is small. Compression adds complexity for negligible gain at this scale. |
