package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/narasaka/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newRepairCmd() *cli.Command {
	return &cli.Command{
		Name:  "repair",
		Usage: "Truncate a corrupted .xhist file at the last valid record",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Usage: "Report without modifying"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)

			xhp, _, err := resolveWorkspaceReadOnly(cmd)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
			}

			f, err := os.Open(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening %s: %v", xhp, err))
			}

			fi, err := f.Stat()
			if err != nil {
				f.Close()
				return outputErrorTo(errW, fmt.Sprintf("stat %s: %v", xhp, err))
			}
			totalSize := fi.Size()

			rd, err := format.NewReader(f)
			if err != nil {
				f.Close()
				return outputErrorTo(errW, fmt.Sprintf("reading %s: %v", xhp, err))
			}

			var records int
			lastValidOffset := int64(format.PreambleSize)
			for {
				_, err := rd.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					break
				}
				records++
			}
			f.Close()

			// Re-scan to find exact byte offset of valid end
			f2, err := os.Open(xhp)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("opening %s: %v", xhp, err))
			}
			rd2, err := format.NewReader(f2)
			if err != nil {
				f2.Close()
				return outputErrorTo(errW, fmt.Sprintf("reading %s: %v", xhp, err))
			}
			validRecords := 0
			for i := 0; i < records; i++ {
				_, err := rd2.Next()
				if err != nil {
					break
				}
				validRecords++
			}
			// Get current position by seeking
			pos, _ := f2.Seek(0, io.SeekCurrent)
			lastValidOffset = pos
			f2.Close()

			bytesToRemove := totalSize - lastValidOffset
			recordsLost := 0
			if bytesToRemove > 0 {
				// Count total records including broken ones
				f3, _ := os.Open(xhp)
				if f3 != nil {
					fi3, _ := f3.Stat()
					_ = fi3
					f3.Close()
				}
			}

			result := map[string]any{
				"truncated_at":  lastValidOffset,
				"records_kept":  validRecords,
				"records_lost":  recordsLost,
				"bytes_removed": bytesToRemove,
			}

			outW := cmdOut(cmd)

			if cmd.Bool("dry-run") {
				return outputJSON(outW, result)
			}

			if bytesToRemove > 0 {
				if err := os.Truncate(xhp, lastValidOffset); err != nil {
					return outputErrorTo(errW, fmt.Sprintf("truncating: %v", err))
				}
			}

			return outputJSON(outW, result)
		},
	}
}
