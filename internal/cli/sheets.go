package cli

import (
	"context"
	"fmt"

	"github.com/narasaka/xhist/internal/excel"
	"github.com/urfave/cli/v3"
)

func newSheetsCmd() *cli.Command {
	return &cli.Command{
		Name:  "sheets",
		Usage: "List sheets in the Excel file",
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}

			sheets, err := excel.ListSheets(xlsxPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("listing sheets: %v", err))
			}

			result := make([]map[string]any, len(sheets))
			for i, s := range sheets {
				result[i] = map[string]any{
					"name": s.Name,
					"rows": s.Rows,
					"cols": s.Cols,
				}
			}
			return outputJSON(cmdOut(cmd), result)
		},
	}
}
