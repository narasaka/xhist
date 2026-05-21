# xhist

Append-only operation log for Excel files. Every read, write, comment, and confusion/resolution an AI agent performs on a spreadsheet is recorded to a workspace-level binary history file. This allows agents to replay their context and maintain a unified timeline across multiple files.

One `.xhist` file tracks all `.xlsx` files in a workspace. The agent never touches the spreadsheet directly. All operations flow through `xhist`.

## Install

```bash
go install github.com/narasaka/xhist/cmd/xhist@latest
```

## Quick Start

```bash
# 1. Initialize a workspace
xhist init my-project

# 2. Perform operations (workspace log is auto-discovered)
xhist write budget.xlsx Sheet1!A1 "Revenue" -m "Adding header"
xhist read budget.xlsx Sheet1!A1
xhist confuse raise budget.xlsx Sheet1!B2 --archetype GAP --headline "Missing value" --description "No source value found" --evidence '[{"id":"ev-b2","sourceRef":{"sourceId":"budget.xlsx","locator":{"kind":"xlsx","sheet":"Sheet1","range":"B2"}},"tone":"focus","label":"Sheet1 B2"}]'

# 3. View the unified timeline
xhist log
```

## Workspace Workflow

xhist is designed for portability. The `{workspace}.xhist` file is the distributable artifact that contains the full audit trail and context for all Excel files in the directory.

1. **Initialize**: Run `xhist init` to start tracking a directory.
2. **Work**: Agents use `xhist read` and `xhist write`.
3. **Distribute**: Copy the `.xlsx` files along with the `.xhist` file to another machine. The history remains intact and searchable.
4. **Recall**: New agent sessions use `xhist log` to understand what has already been done across all files.

## Usage

```bash
xhist init [name]
xhist write <file.xlsx> <range> [value] -m "message"
xhist read <file.xlsx> <range>
xhist log [file.xlsx]
xhist confuse raise <file.xlsx> <cell> --archetype <type> --headline <text> --description <text> --evidence <json-array>
xhist confuse resolve <id> --value <value> --confidence high --reasoning <text>
xhist info
```

Run `xhist --help` for the full command list.

## Development

Requires Go 1.24+.

```bash
git clone https://github.com/narasaka/xhist.git
cd xhist
make build
make test
```

Available make targets:

```
make build      Build the binary to bin/xhist
make test       Run all tests with race detector
make lint       Run gofmt check and go vet
make install    Install to GOPATH/bin
make clean      Remove build artifacts
```

## Agent Skill

Install the xhist skill so your AI agent knows how to use the CLI:

```bash
npx skills add https://github.com/narasaka/xhist --skill xhist
```
