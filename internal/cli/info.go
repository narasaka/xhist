package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func newInfoCmd() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show metadata and stats about the history file",
		Action: func(_ context.Context, cmd *cli.Command) error {
			errW := cmdErr(cmd)

			xhp, wsRoot, err := resolveWorkspaceReadOnly(cmd)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("workspace: %v", err))
			}

			var fileFilter string
			if xlsxArg := cmd.Args().Get(0); xlsxArg != "" {
				fileFilter, err = resolveTargetFile(wsRoot, xlsxArg)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("resolving file: %v", err))
				}
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

			var (
				workspaceName  string
				created        int64
				ops            int
				reads          int
				writes         int
				commentSets    int
				commentGets    int
				commentDeletes int
				lastOpTS       int64
				sheetsMap      = map[string]bool{}
				filesMap       = map[string]bool{}
				hasFooter      bool
				metadata       = map[string]string{}
			)

			for {
				rec, err := rd.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					return outputErrorTo(errW, fmt.Sprintf("reading record: %v", err))
				}
				switch v := rec.Parsed.(type) {
				case format.Header:
					workspaceName = v.WorkspaceName
					if workspaceName == "" {
						workspaceName = v.TargetFile
					}
					created = v.CreatedAt
				case format.Op:
					if v.TargetFile != "" {
						filesMap[v.TargetFile] = true
					}
					if fileFilter != "" && v.TargetFile != fileFilter {
						continue
					}
					ops++
					if v.Action == format.ActionRead {
						reads++
					} else {
						writes++
					}
					if v.Timestamp > lastOpTS {
						lastOpTS = v.Timestamp
					}
					if v.Sheet != "" {
						sheetsMap[v.Sheet] = true
					}
				case format.CommentOp:
					if v.TargetFile != "" {
						filesMap[v.TargetFile] = true
					}
					if fileFilter != "" && v.TargetFile != fileFilter {
						continue
					}
					switch v.Action {
					case format.ActionCommentSet:
						commentSets++
					case format.ActionCommentGet:
						commentGets++
					case format.ActionCommentDelete:
						commentDeletes++
					}
					if v.Timestamp > lastOpTS {
						lastOpTS = v.Timestamp
					}
					if v.Sheet != "" {
						sheetsMap[v.Sheet] = true
					}
				case format.Metadata:
					metadata[v.Key] = v.Value
				case format.Footer:
					hasFooter = true
				}
			}

			sheets := make([]string, 0, len(sheetsMap))
			for s := range sheetsMap {
				sheets = append(sheets, s)
			}

			files := make([]string, 0, len(filesMap))
			for f := range filesMap {
				files = append(files, f)
			}

			idxPath := xhp + ".idx"
			indexStale := false
			if _, err := os.Stat(idxPath); err == nil {
				stale, err := format.IsStale(idxPath, xhp)
				if err == nil {
					indexStale = stale
				}
			} else {
				indexStale = true
			}

			result := map[string]any{
				"workspace":      workspaceName,
				"created":        time.UnixMilli(created).UTC().Format(time.RFC3339),
				"ops":            ops,
				"reads":          reads,
				"writes":         writes,
				"sheets_touched": sheets,
				"files":          files,
				"has_footer":     hasFooter,
				"index_stale":    indexStale,
				"metadata":       metadata,
			}
			if lastOpTS > 0 {
				result["last_op"] = time.UnixMilli(lastOpTS).UTC().Format(time.RFC3339)
			}
			if commentSets+commentGets+commentDeletes > 0 {
				result["comment_sets"] = commentSets
				result["comment_gets"] = commentGets
				result["comment_deletes"] = commentDeletes
			}
			return outputJSON(cmdOut(cmd), result)
		},
	}
}
