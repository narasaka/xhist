package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/prosights/xhist/internal/format"
	"github.com/urfave/cli/v3"
)

func outputJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func outputNDJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func outputError(msg string) error {
	b, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintln(os.Stderr, string(b))
	return cli.Exit(msg, 1)
}

func outputErrorTo(w io.Writer, msg string) error {
	b, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintln(w, string(b))
	return cli.Exit(msg, 1)
}

// cmdOut returns the root command's Writer, falling back to os.Stdout.
func cmdOut(cmd *cli.Command) io.Writer {
	if w := cmd.Root().Writer; w != nil {
		return w
	}
	return os.Stdout
}

// cmdErr returns the root command's ErrWriter, falling back to os.Stderr.
func cmdErr(cmd *cli.Command) io.Writer {
	if w := cmd.Root().ErrWriter; w != nil {
		return w
	}
	return os.Stderr
}

func cellToJSON(c format.Cell) any {
	switch c.Type {
	case format.CellString:
		return c.String
	case format.CellNumber:
		return c.Number
	case format.CellBool:
		return c.Bool
	case format.CellFormula:
		return "=" + c.FormulaText
	case format.CellError:
		return c.Error
	default:
		return nil
	}
}

func jsonToCell(v any) format.Cell {
	switch val := v.(type) {
	case string:
		if len(val) > 0 && val[0] == '=' {
			return format.Cell{Type: format.CellFormula, FormulaText: val[1:]}
		}
		return format.Cell{Type: format.CellString, String: val}
	case float64:
		return format.Cell{Type: format.CellNumber, Number: val}
	case bool:
		return format.Cell{Type: format.CellBool, Bool: val}
	default:
		return format.Cell{Type: format.CellEmpty}
	}
}

func gridToJSON(cells [][]format.Cell) [][]any {
	rows := make([][]any, len(cells))
	for r, row := range cells {
		rows[r] = make([]any, len(row))
		for c, cell := range row {
			rows[r][c] = cellToJSON(cell)
		}
	}
	return rows
}
