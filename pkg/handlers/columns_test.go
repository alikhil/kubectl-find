package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_isBuiltin(t *testing.T) {
	v1.AddToScheme(scheme.Scheme)
	tests := []struct {
		name         string
		resourceType schema.GroupVersionKind
		want         bool
	}{
		{
			name:         "Pod is builtin",
			resourceType: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			want:         true,
		},
		{
			name:         "Service is builtin",
			resourceType: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
			want:         true,
		},
		{
			name: "Certificate from cert-manager is not builtin",
			resourceType: schema.GroupVersionKind{
				Group:   "cert-manager.io",
				Version: "v1",
				Kind:    "Certificate",
			},
			want: false,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			got := isBuiltin(scheme, tt.resourceType)
			if got != tt.want {
				t.Errorf("isBuiltin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_GetAnnotationColumns(t *testing.T) {
	tests := []struct {
		name        string
		opts        HandlerOptions
		obj         unstructured.Unstructured
		wantHeaders []string
		wantValues  []string
	}{
		{
			name: "No annotations requested",
			opts: HandlerOptions{
				annotations: nil,
			},
			obj:         unstructured.Unstructured{},
			wantHeaders: []string{},
			wantValues:  []string{},
		},
		{
			name: "Single annotation with value",
			opts: HandlerOptions{
				annotations: []string{"example.com/annotation"},
			},
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"example.com/annotation": "test-value",
						},
					},
				},
			},
			wantHeaders: []string{"ANNOTATION"},
			wantValues:  []string{"test-value"},
		},
		{
			name: "Single annotation without value",
			opts: HandlerOptions{
				annotations: []string{"example.com/annotation"},
			},
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{},
					},
				},
			},
			wantHeaders: []string{"ANNOTATION"},
			wantValues:  []string{NoneStr},
		},
		{
			name: "Multiple annotations",
			opts: HandlerOptions{
				annotations: []string{"example.com/annotation1", "annotation2"},
			},
			obj: unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"example.com/annotation1": "value1",
							"annotation2":             "value2",
						},
					},
				},
			},
			wantHeaders: []string{"ANNOTATION1", "ANNOTATION2"},
			wantValues:  []string{"value1", "value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			columns := GetAnnotationColumns(tt.opts)
			
			require.Equal(t, len(tt.wantHeaders), len(columns), "Number of columns should match")
			
			for i, col := range columns {
				require.Equal(t, tt.wantHeaders[i], col.Header, "Column header should match")
				require.Equal(t, tt.wantValues[i], col.Value(tt.obj), "Column value should match")
			}
		})
	}
}

