package printers

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestOutputPrinter(t *testing.T) {
	t.Parallel()

	objects := []unstructured.Unstructured{{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "demo",
			"namespace": "default",
		},
	}}}

	tests := []struct {
		format string
		want   string
	}{
		{
			format: "json",
			want: `{
    "apiVersion": "v1",
    "items": [
        {
            "apiVersion": "v1",
            "kind": "Pod",
            "metadata": {
                "name": "demo",
                "namespace": "default"
            }
        }
    ],
    "kind": "PodList"
}
`,
		},
		{
			format: "yaml",
			want: `apiVersion: v1
items:
- apiVersion: v1
  kind: Pod
  metadata:
    name: demo
    namespace: default
kind: PodList
`,
		},
		{
			format: "kyaml",
			want: `---
{
  apiVersion: "v1",
  items: [{
    apiVersion: "v1",
    kind: "Pod",
    metadata: {
      name: "demo",
      namespace: "default",
    },
  }],
  kind: "PodList",
}
`,
		},
		{format: "name", want: "pod/demo\n"},
		{format: "go-template={{range .items}}{{.metadata.name}}{{end}}", want: "demo"},
		{format: "gotemplate={{range .items}}{{.metadata.name}}{{end}}", want: "demo"},
		{format: "jsonpath={.items[*].metadata.name}", want: "demo"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			printer, printerErr := NewOutputPrinter(tt.format, schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
			require.NoError(t, printerErr)

			output := &bytes.Buffer{}
			require.NoError(t, printer.PrintObjects(objects, output))
			require.Equal(t, tt.want, output.String())
		})
	}
}

func TestNewOutputPrinterRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()

	_, err := NewOutputPrinter("wide", schema.GroupVersionKind{})
	require.EqualError(
		t,
		err,
		`unsupported output format "wide"; supported formats are json, yaml, kyaml, name, go-template=EXPR, and jsonpath=EXPR`,
	)
}
