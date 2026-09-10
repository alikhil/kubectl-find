package handlers

import (
	"bytes"
	"testing"

	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewBatchPrinterDefaultsToTableOutput(t *testing.T) {
	t.Parallel()

	printer, err := newBatchPrinter(HandlerOptions{}, Resource{GroupVersionResource: PodType})
	require.NoError(t, err)
	_, isTablePrinter := printer.(*printers.TablePrinter)
	require.True(t, isTablePrinter)
}

func TestNewBatchPrinterUsesConfiguredOutputFormat(t *testing.T) {
	t.Parallel()

	printer, err := newBatchPrinter(HandlerOptions{output: "json"}, Resource{GroupVersionResource: PodType})
	require.NoError(t, err)
	_, isTablePrinter := printer.(*printers.TablePrinter)
	require.False(t, isTablePrinter)
}

func TestOutputPrinterAddsTypeMetadataForTypedResources(t *testing.T) {
	t.Parallel()

	printer, err := newBatchPrinter(
		HandlerOptions{output: "name"},
		Resource{
			GroupVersionResource: PodType,
			GroupVersionKind:     schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
		},
	)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	err = printer.PrintObjects([]unstructured.Unstructured{{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "demo"},
	}}}, output)
	require.NoError(t, err)
	require.Equal(t, "pod/demo\n", output.String())
}
