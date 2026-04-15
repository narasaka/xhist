# xhist

Append-only operation log for Excel files. Every read and write an AI agent performs on a spreadsheet is recorded to a binary history file, so the agent can replay its context without relying on its context window.

One `.xhist` file tracks exactly one `.xlsx` file. The agent never touches the spreadsheet directly -- all operations go through `xhist`.

## Install

```
go install github.com/prosights/xhist/cmd/xhist@latest
```

## Usage

```
xhist init budget.xlsx
xhist write budget.xlsx Sheet1!A1 "Revenue" -m "Adding header"
xhist read budget.xlsx Sheet1!A1
xhist log budget.xlsx
```

Run `xhist --help` for the full command list.

## Development

Requires Go 1.24+.

```
git clone https://github.com/prosights/xhist.git
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
