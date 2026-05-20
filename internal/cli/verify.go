package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/narasaka/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newVerifyCmd() *cli.Command {
	return &cli.Command{
		Name:  "verify",
		Usage: "Check integrity of the .xhist file",
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			outW := cmdOut(cmd)

			xhp, _, err := resolveWorkspaceReadOnly(cmd)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
			}

			f, err := os.Open(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening %s: %v", xhp, err))
			}
			defer f.Close()

			rd, err := format.NewReader(f)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("reading %s: %v", xhp, err))
			}

			var records, ops int
			for {
				rec, err := rd.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					if corr, ok := err.(*format.ErrCorruption); ok {
						result := map[string]any{
							"ok":                false,
							"records_valid":     records,
							"corruption_offset": corr.Offset,
							"error":             corr.Error(),
						}
						outputJSON(outW, result)
						return cli.Exit("corruption detected", 1)
					}
					return outputErrorTo(errW, fmt.Sprintf("reading record: %v", err))
				}
				records++
				if rec.Opcode == format.OpcodeOp {
					ops++
				}
			}

			return outputJSON(outW, map[string]any{
				"ok":      true,
				"records": records,
				"ops":     ops,
			})
		},
	}
}
