package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"

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
