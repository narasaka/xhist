package cli

import (
	"context"

	"github.com/urfave/cli/v3"
)

func buildRootCommand() *cli.Command {
	return &cli.Command{
		Name:    "xhist",
		Usage:   "Git for Excel — append-only operation log for spreadsheets",
		Version: "0.1.0",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "human",
				Aliases: []string{"H"},
				Usage:   "Human-readable output instead of JSON",
			},
		},
		Commands: []*cli.Command{
			newInitCmd(),
			newReadCmd(),
			newWriteCmd(),
			newLogCmd(),
			newShowCmd(),
			newSheetsCmd(),
			newInfoCmd(),
			newStateCmd(),
			newVerifyCmd(),
			newRepairCmd(),
			newReindexCmd(),
		},
	}
}

func Run(ctx context.Context, args []string) error {
	return buildRootCommand().Run(ctx, args)
}
