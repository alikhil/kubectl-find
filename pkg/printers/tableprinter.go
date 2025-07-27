package printers

import (
	"fmt"
	"io"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
)

type TablePrinterOptions struct {
	ShowNamespace     bool
	AdditionalColumns []Column // additional columns to add to the table after NAME
}

type TablePrinter struct {
	options TablePrinterOptions
}

type Column struct {
	Header string
	Value  func(unstructured.Unstructured) string
}

// NewTablePrinter creates a printer suitable for calling PrintObjects().
func NewTablePrinter(options TablePrinterOptions) BatchPrinter {
	printer := &TablePrinter{
		options: options,
	}
	return printer
}

func (p *TablePrinter) PrintObjects(objects []unstructured.Unstructured, out io.Writer) error {
	if len(objects) == 0 {
		return nil // nothing to print
	}

	table := tablewriter.NewTable(out,
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

	columns := []Column{}

	if p.options.ShowNamespace {
		columns = append(columns, Column{
			Header: "NAMESPACE",
			Value: func(obj unstructured.Unstructured) string {
				return obj.GetNamespace()
			},
		})
	}

	columns = append(columns, Column{
		Header: "NAME",
		Value: func(obj unstructured.Unstructured) string {
			return obj.GetName()
		},
	})

	columns = append(columns, p.options.AdditionalColumns...)

	columns = append(columns, Column{
		Header: "AGE",
		Value: func(obj unstructured.Unstructured) string {
			timestamp := obj.GetCreationTimestamp()
			if timestamp.IsZero() {
				return "<unknown>"
			}

			return duration.HumanDuration(time.Since(timestamp.Time))
		},
	})

	headers := make([]string, len(columns))
	for i := range columns {
		headers[i] = columns[i].Header
	}

	data := make([][]string, len(objects))
	for i, obj := range objects {
		row := make([]string, len(columns))
		for j, col := range columns {
			row[j] = col.Value(obj)
		}
		data[i] = row
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
