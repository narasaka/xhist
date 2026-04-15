package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prosights/xhist/internal/excel"
	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newInitCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create a new .xhist file for an Excel file",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "Write agent.name metadata"},
			&cli.StringFlag{Name: "model", Usage: "Write agent.model metadata"},
			&cli.StringFlag{Name: "session", Usage: "Write session.id metadata"},
			&cli.BoolFlag{Name: "force", Usage: "Overwrite existing .xhist file"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)
			xlsxPath := cmd.Args().Get(0)
			if xlsxPath == "" {
				return outputErrorTo(errW, "missing required argument: <file.xlsx>")
			}

			xhp := xhistPath(xlsxPath)

			if _, err := os.Stat(xhp); err == nil && !cmd.Bool("force") {
				return outputErrorTo(errW, fmt.Sprintf("%s already exists (use --force to overwrite)", xhp))
			}

			if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
				if err := excel.CreateWorkbook(xlsxPath); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("creating workbook: %v", err))
				}
			}

			f, err := os.Create(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("creating %s: %v", xhp, err))
			}
			defer f.Close()

			w, err := format.NewWriter(f)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing preamble: %v", err))
			}

			if err := w.WriteHeader(time.Now().UnixMilli(), filepath.Base(xlsxPath)); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing header: %v", err))
			}

			if v := cmd.String("agent"); v != "" {
				if err := w.WriteMetadata("agent.name", v); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("writing metadata: %v", err))
				}
			}
			if v := cmd.String("model"); v != "" {
				if err := w.WriteMetadata("agent.model", v); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("writing metadata: %v", err))
				}
			}
			if v := cmd.String("session"); v != "" {
				if err := w.WriteMetadata("session.id", v); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("writing metadata: %v", err))
				}
			}

			return outputJSON(cmdOut(cmd), map[string]string{"created": xhp})
		},
	}
}
