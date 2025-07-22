package pkg

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

//go:generate ../.devenv/state/go/bin/mockgen -source $GOFILE -destination ./mocks/$GOFILE -package=mocks

type Printer interface {
	Print(headers []string, data [][]string) error
}

type PrettyPrinter struct {
	out io.Writer
}

// NewPrettyPrinter creates a new PrettyPrinter that writes to the specified output file.
func NewPrettyPrinter(out io.Writer) *PrettyPrinter {
	if out == nil {
		out = os.Stdout // default to stdout if no output file is provided
	}
	return &PrettyPrinter{out: out}
}

func (pp *PrettyPrinter) Print(headers []string, data [][]string) error {

	table := tablewriter.NewTable(pp.out,

		// tell render not to render any lines and separators
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Borders: tw.BorderNone,
			Settings: tw.Settings{
				Separators: tw.SeparatorsNone,
				Lines:      tw.LinesNone,
			},
		})),

		// Set general configuration
		tablewriter.WithConfig(
			tablewriter.Config{
				Header: tw.CellConfig{
					Formatting: tw.CellFormatting{
						Alignment:  tw.AlignLeft, // force alignment for header
						AutoFormat: tw.Off,
					},
					Padding: tw.CellPadding{Global: tw.Padding{Right: "   "}},
				},
				Row: tw.CellConfig{
					Formatting: tw.CellFormatting{
						Alignment: tw.AlignLeft, // force alightment for body
					},

					// remove all padding in a in all cells
					Padding: tw.CellPadding{Global: tw.Padding{Right: "   "}},
				},
			},
		),
	)
	for i := range headers {
		headers[i] = strings.ToUpper(headers[i])
	}
	table.Header(headers)
	err := table.Bulk(data)
	if err != nil {
		return fmt.Errorf("failed to add data to table: %w", err)
	}
	err = table.Render()
	if err != nil {
		return fmt.Errorf("failed to render table: %w", err)
	}
	return nil
}
