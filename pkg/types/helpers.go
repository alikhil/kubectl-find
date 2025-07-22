package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func getNestedColumn(t *testing.T, obj unstructured.Unstructured, column ...string) string {
	t.Helper()
	columns, found, err := unstructured.NestedString(obj.Object, column...)
	if err != nil || !found {
		t.Fatalf("Failed to get nested column %v: %v", column, err)
	}
	return columns
}

func toUL(t *testing.T, objs ...runtime.Object) []unstructured.Unstructured {
	t.Helper()
	var ul []unstructured.Unstructured
	for _, obj := range objs {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		require.NoError(t, err)
		ul = append(ul, unstructured.Unstructured{Object: u})
	}
	return ul
}

func getResource(resourceType string) Resource {
	switch resourceType {
	case "configmap":
		return Resource{
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
			PluralName:   "configmaps",
			SingularName: "configmap",
			IsNamespaced: true,
		}
	case "secret":
		return Resource{
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "secrets",
			},
			PluralName:   "secrets",
			SingularName: "secret",
			IsNamespaced: true,
		}
	case "namespace":
		return Resource{
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			PluralName:   "namespaces",
			SingularName: "namespace",
			IsNamespaced: false,
		}
	default:
		return Resource{}
	}
}
