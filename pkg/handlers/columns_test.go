package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func toUnstructured(t *testing.T, obj runtime.Object) unstructured.Unstructured {
	t.Helper()

	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	require.NoError(t, err)

	return unstructured.Unstructured{Object: raw}
}

func Test_GetColumnsForDeployments(t *testing.T) {
	t.Helper()

	replicas := int32(3)
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     2,
			UpdatedReplicas:   3,
			AvailableReplicas: 1,
		},
	}

	columns := GetColumnsFor(HandlerOptions{}, Resource{GroupVersionResource: DeploymentType})
	require.Len(t, columns, 3)

	obj := toUnstructured(t, deployment)

	require.Equal(t, "READY", columns[0].Header)
	require.Equal(t, "2/3", columns[0].Value(obj))
	require.Equal(t, "UP-TO-DATE", columns[1].Header)
	require.Equal(t, "3", columns[1].Value(obj))
	require.Equal(t, "AVAILABLE", columns[2].Header)
	require.Equal(t, "1", columns[2].Value(obj))
}

func Test_GetColumnsForStatefulSets(t *testing.T) {
	t.Helper()

	replicas := int32(4)
	statefulSet := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 2,
		},
	}

	columns := GetColumnsFor(HandlerOptions{}, Resource{GroupVersionResource: StatefulSetType})
	require.Len(t, columns, 1)

	obj := toUnstructured(t, statefulSet)

	require.Equal(t, "READY", columns[0].Header)
	require.Equal(t, "2/4", columns[0].Value(obj))
}

func Test_GetColumnsForReplicaSets(t *testing.T) {
	t.Helper()

	replicas := int32(5)
	replicaSet := &appsv1.ReplicaSet{
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:      4,
			ReadyReplicas: 3,
		},
	}

	columns := GetColumnsFor(HandlerOptions{}, Resource{GroupVersionResource: ReplicaSetType})
	require.Len(t, columns, 3)

	obj := toUnstructured(t, replicaSet)

	require.Equal(t, "DESIRED", columns[0].Header)
	require.Equal(t, "5", columns[0].Value(obj))
	require.Equal(t, "CURRENT", columns[1].Header)
	require.Equal(t, "4", columns[1].Value(obj))
	require.Equal(t, "READY", columns[2].Header)
	require.Equal(t, "3", columns[2].Value(obj))
}

func Test_GetColumnsForDaemonSets(t *testing.T) {
	t.Helper()

	daemonSet := &appsv1.DaemonSet{
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 10,
			CurrentNumberScheduled: 8,
			NumberReady:            7,
			UpdatedNumberScheduled: 6,
			NumberAvailable:        5,
		},
	}

	columns := GetColumnsFor(HandlerOptions{}, Resource{GroupVersionResource: DaemonSetType})
	require.Len(t, columns, 5)

	obj := toUnstructured(t, daemonSet)

	require.Equal(t, "DESIRED", columns[0].Header)
	require.Equal(t, "10", columns[0].Value(obj))
	require.Equal(t, "CURRENT", columns[1].Header)
	require.Equal(t, "8", columns[1].Value(obj))
	require.Equal(t, "READY", columns[2].Header)
	require.Equal(t, "7", columns[2].Value(obj))
	require.Equal(t, "UP-TO-DATE", columns[3].Header)
	require.Equal(t, "6", columns[3].Value(obj))
	require.Equal(t, "AVAILABLE", columns[4].Header)
	require.Equal(t, "5", columns[4].Value(obj))
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

			require.Len(t, columns, len(tt.wantHeaders), "Number of columns should match")

			for i, col := range columns {
				require.Equal(t, tt.wantHeaders[i], col.Header, "Column header should match")
				require.Equal(t, tt.wantValues[i], col.Value(tt.obj), "Column value should match")
			}
		})
	}
}
