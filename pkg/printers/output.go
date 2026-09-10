package printers

import (
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/printers"
)

const (
	nameOutputFormat = "name"
	listKindSuffix   = "List"
)

// ValidateOutputFormat checks whether format is supported and whether any
// template or JSONPath expression is valid.
func ValidateOutputFormat(format string) error {
	if format == "" {
		return nil
	}
	_, err := newResourcePrinter(format)
	return err
}

// NewOutputPrinter creates a printer for a kubectl-compatible output format.
// Go templates and JSONPath expressions must be passed as go-template=EXPR
// (or gotemplate=EXPR) and jsonpath=EXPR respectively.
func NewOutputPrinter(format string, resourceGVK schema.GroupVersionKind) (BatchPrinter, error) {
	resourcePrinter, err := newResourcePrinter(format)
	if err != nil {
		return nil, err
	}
	return &outputPrinter{resourcePrinter: resourcePrinter, resourceGVK: resourceGVK}, nil
}

func newResourcePrinter(format string) (printers.ResourcePrinter, error) {
	var resourcePrinter printers.ResourcePrinter

	switch {
	case format == "json":
		resourcePrinter = &printers.JSONPrinter{}
	case format == "yaml":
		resourcePrinter = &printers.YAMLPrinter{}
	case format == "kyaml":
		resourcePrinter = &printers.KYAMLPrinter{}
	case format == nameOutputFormat:
		resourcePrinter = &printers.NamePrinter{}
	case strings.HasPrefix(format, "go-template="):
		var err error
		resourcePrinter, err = printers.NewGoTemplatePrinter([]byte(strings.TrimPrefix(format, "go-template=")))
		if err != nil {
			return nil, fmt.Errorf("invalid go-template output format: %w", err)
		}
	case strings.HasPrefix(format, "gotemplate="):
		var err error
		resourcePrinter, err = printers.NewGoTemplatePrinter([]byte(strings.TrimPrefix(format, "gotemplate=")))
		if err != nil {
			return nil, fmt.Errorf("invalid gotemplate output format: %w", err)
		}
	case strings.HasPrefix(format, "jsonpath="):
		var err error
		resourcePrinter, err = printers.NewJSONPathPrinter(strings.TrimPrefix(format, "jsonpath="))
		if err != nil {
			return nil, fmt.Errorf("invalid jsonpath output format: %w", err)
		}
	default:
		return nil, fmt.Errorf(
			"unsupported output format %q; supported formats are json, yaml, kyaml, name, go-template=EXPR, and jsonpath=EXPR",
			format,
		)
	}

	return resourcePrinter, nil
}

type outputPrinter struct {
	resourcePrinter printers.ResourcePrinter
	resourceGVK     schema.GroupVersionKind
}

func (p *outputPrinter) PrintObjects(objects []unstructured.Unstructured, out io.Writer) error {
	listGVK := p.resourceGVK
	if listGVK.Kind != "" {
		listGVK.Kind += listKindSuffix
	} else {
		listGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: listKindSuffix}
	}
	for i := range objects {
		if objects[i].GroupVersionKind().Kind == "" && p.resourceGVK.Kind != "" {
			objects[i].SetGroupVersionKind(p.resourceGVK)
		}
	}
	list := &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       listKindSuffix,
		},
		Items: objects,
	}
	list.SetGroupVersionKind(listGVK)

	if err := p.resourcePrinter.PrintObj(list, out); err != nil {
		return fmt.Errorf("print objects: %w", err)
	}
	return nil
}
