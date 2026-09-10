package handlers

import "github.com/alikhil/kubectl-find/pkg/printers"

func newBatchPrinter(opts HandlerOptions, resource Resource) (printers.BatchPrinter, error) {
	if opts.output != "" {
		return printers.NewOutputPrinter(opts.output, resource.GroupVersionKind)
	}

	return printers.NewTablePrinter(printers.TablePrinterOptions{
		ShowNamespace:     resource.IsNamespaced && opts.allNamespaces,
		AdditionalColumns: GetColumnsFor(opts, resource),
		SuffixColumns:     GetSuffixColumnsFor(resource),
		LabelColumns:      GetLabelColumns(opts, resource.GroupVersionResource),
		AnnotationColumns: GetAnnotationColumns(opts),
	}), nil
}
