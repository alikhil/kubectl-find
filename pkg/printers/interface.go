package printers

import (
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

//go:generate ../../.devenv/state/go/bin/mockgen -source $GOFILE -destination ../mocks/batchprinter.go -package=mocks

type BatchPrinter interface {
	PrintObjects([]unstructured.Unstructured, io.Writer) error
}
