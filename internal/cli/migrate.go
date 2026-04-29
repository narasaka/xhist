package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/prosights/xhist/internal/format"
	"github.com/prosights/xhist/internal/workspace"
	"github.com/urfave/cli/v3"
)

type mergeEntry struct {
	targetFile string
	timestamp  int64
	sequence   uint32
	isComment  bool
	op         format.Op
	commentOp  format.CommentOp
}

func newMigrateCmd() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Consolidate v1 per-file .xhist logs into a single v2 workspace file",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Usage: "Workspace name (default: directory name)"},
			&cli.StringFlag{Name: "dir", Usage: "Directory to scan for v1 files (default: cwd)"},
			&cli.BoolFlag{Name: "dry-run", Usage: "Preview migration without writing"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			outW := cmdOut(cmd)
			errW := cmdErr(cmd)

			scanDir := cmd.String("dir")
			if scanDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("getting cwd: %v", err))
				}
				scanDir = cwd
			}
			scanDir, err := filepath.Abs(scanDir)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("resolving dir: %v", err))
			}

			name := cmd.String("name")
			if name == "" {
				name = workspace.DefaultName(scanDir)
			}

			var outputPath string
			if ws := cmd.Root().String("workspace"); ws != "" {
				if !strings.HasSuffix(ws, ".xhist") {
					return outputErrorTo(errW, fmt.Sprintf("workspace path must end with .xhist: %s", ws))
				}
				abs, err := filepath.Abs(ws)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("resolving workspace path: %v", err))
				}
				outputPath = abs
			} else {
				outputPath = filepath.Join(scanDir, name+".xhist")
			}

			if _, err := os.Stat(outputPath); err == nil {
				return outputErrorTo(errW, fmt.Sprintf("output file already exists: %s", outputPath))
			}

			var v1Files []string
			err = filepath.WalkDir(scanDir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".xhist") {
					return nil
				}
				if strings.HasSuffix(path, ".xhist.idx") || strings.HasSuffix(path, ".xhist.lock") {
					return nil
				}
				abs, _ := filepath.Abs(path)
				if abs == outputPath {
					return nil
				}
				if isV1File(path) {
					v1Files = append(v1Files, path)
				}
				return nil
			})
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("scanning directory: %v", err))
			}

			if len(v1Files) == 0 {
				return outputErrorTo(errW, "no v1 .xhist files found")
			}

			var entries []mergeEntry
			var sourceFiles []string

			for _, v1Path := range v1Files {
				ops, _, err := readV1Ops(v1Path)
				if err != nil {
					return outputErrorTo(errW, fmt.Sprintf("reading %s: %v", v1Path, err))
				}

				rel, err := filepath.Rel(scanDir, v1Path)
				if err != nil {
					rel = v1Path
				}
				sourceFiles = append(sourceFiles, filepath.ToSlash(rel))
				entries = append(entries, ops...)
			}

			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].timestamp != entries[j].timestamp {
					return entries[i].timestamp < entries[j].timestamp
				}
				if entries[i].targetFile != entries[j].targetFile {
					return entries[i].targetFile < entries[j].targetFile
				}
				return entries[i].sequence < entries[j].sequence
			})

			sort.Strings(sourceFiles)

			if cmd.Bool("dry-run") {
				return outputJSON(outW, map[string]any{
					"dry_run":      true,
					"files_found":  len(v1Files),
					"ops_total":    len(entries),
					"source_files": sourceFiles,
				})
			}

			f, err := os.Create(outputPath)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("creating output: %v", err))
			}
			defer f.Close()

			w, err := format.NewWriter(f)
			if err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing preamble: %v", err))
			}

			if err := w.WriteHeader(time.Now().UnixMilli(), name); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing header: %v", err))
			}

			for i, entry := range entries {
				seq := uint32(i + 1)
				if entry.isComment {
					cop := entry.commentOp
					cop.Sequence = seq
					if err := w.WriteCommentOp(cop); err != nil {
						return outputErrorTo(errW, fmt.Sprintf("writing comment op: %v", err))
					}
				} else {
					op := entry.op
					op.Sequence = seq
					if err := w.WriteOp(op); err != nil {
						return outputErrorTo(errW, fmt.Sprintf("writing op: %v", err))
					}
				}
			}

			opCount := uint32(len(entries))
			if err := w.WriteFooter(opCount, opCount); err != nil {
				return outputErrorTo(errW, fmt.Sprintf("writing footer: %v", err))
			}

			return outputJSON(outW, map[string]any{
				"created":      outputPath,
				"files_merged": len(v1Files),
				"ops_migrated": len(entries),
				"source_files": sourceFiles,
			})
		},
	}
}

func isV1File(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	var buf [format.PreambleSize]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return false
	}
	for i := 0; i < 6; i++ {
		if buf[i] != format.Magic[i] {
			return false
		}
	}
	return buf[6] == format.VersionV1
}

func readV1Ops(path string) ([]mergeEntry, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	rd, err := format.NewReader(f)
	if err != nil {
		return nil, "", err
	}

	var entries []mergeEntry
	var targetFile string

	for {
		rec, err := rd.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return entries, targetFile, nil
		}
		switch v := rec.Parsed.(type) {
		case format.Header:
			targetFile = v.TargetFile
		case format.Op:
			entries = append(entries, mergeEntry{
				targetFile: v.TargetFile,
				timestamp:  v.Timestamp,
				sequence:   v.Sequence,
				isComment:  false,
				op:         v,
			})
		case format.CommentOp:
			entries = append(entries, mergeEntry{
				targetFile: v.TargetFile,
				timestamp:  v.Timestamp,
				sequence:   v.Sequence,
				isComment:  true,
				commentOp:  v,
			})
		}
	}

	return entries, targetFile, nil
}
