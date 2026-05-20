package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/narasaka/xhist/internal/format"
	"github.com/narasaka/xhist/internal/workspace"
	"github.com/urfave/cli/v3"
)

func newInitCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create a new workspace .xhist file",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "agent", Usage: "Write agent.name metadata"},
			&cli.StringFlag{Name: "model", Usage: "Write agent.model metadata"},
			&cli.StringFlag{Name: "session", Usage: "Write session.id metadata"},
			&cli.BoolFlag{Name: "force", Usage: "Overwrite existing .xhist file"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)

			name := cmd.Args().Get(0)
			cwd, err := os.Getwd()
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("getting cwd: %v", err))
			}
			if name == "" {
				name = workspace.DefaultName(cwd)
			}

			if ws := cmd.Root().String("workspace"); ws != "" {
				if !strings.HasSuffix(ws, ".xhist") {
					return outputErrorTo(errW, fmt.Sprintf("workspace path must end with .xhist: %s", ws))
				}
				abs, err := filepath.Abs(ws)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("resolving path: %v", err))
				}
				name = strings.TrimSuffix(filepath.Base(abs), ".xhist")
				cwd = filepath.Dir(abs)
			}

			xhp := filepath.Join(cwd, name+".xhist")

			if _, err := os.Stat(xhp); err == nil && !cmd.Bool("force") {
				return outputErrorTo(errW, fmt.Sprintf("%s already exists (use --force to overwrite)", xhp))
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

			if err := w.WriteHeader(time.Now().UnixMilli(), name); err != nil {
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
