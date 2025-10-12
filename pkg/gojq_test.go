package pkg

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMatchesWithGoJQ(t *testing.T) {
	obj := map[string]any{
		"Name": "Alice",
		"Age":  34,
	}
	query, err := PrepareQuery("(.Age > 30) and (.Name == \"Alice\")")
	if err != nil {
		t.Fatal("Failed to prepare query:", err)
	}
	match, err := MatchesWithGoJQ(obj, query)
	if err != nil {
		t.Fatal("Error:", err)
	}
	if !match {
		t.Fatal("Expected match to be true")
	}
}

func TestMatchesWithGoJQPods(t *testing.T) {
	tests := []struct {
		name string
		pod  *v1.Pod
		expr string
		want bool
	}{
		{
			name: "pod with matching label",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						"app": "myapp",
					},
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
				},
			},
			expr: ".metadata.labels.app == \"myapp\" and .status.phase == \"Running\"",
			want: true,
		},
		{
			name: "pod with imagePullSecret",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: v1.PodSpec{
					ImagePullSecrets: []v1.LocalObjectReference{
						{Name: "other-secret"},
						{Name: "my-secret"},
					},
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
				},
			},
			expr: "any(.spec.imagePullSecrets[]; .name == \"my-secret\")",
			want: true,
		},
		{
			name: "pod with imagePullSecrets and pumpumpumpum",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: v1.PodSpec{
					ImagePullSecrets: []v1.LocalObjectReference{
						{Name: "other-secret"},
					},
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
				},
			},
			expr: "any(.spec.imagePullSecrets[]; .name == \"my-secret\" | \"pumpumpumpum\")",
			// this is a jq behavior
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := PrepareQuery(tt.expr)
			if err != nil {
				t.Fatalf("PrepareQuery() error = %v", err)
			}
			pod, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tt.pod)
			if err != nil {
				t.Fatalf("ToUnstructured() error = %v", err)
			}
			got, err := MatchesWithGoJQ(pod, query)
			if err != nil {
				t.Fatalf("MatchesWithGoJQ() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("MatchesWithGoJQ() got = %v, want %v", got, tt.want)
			}
		})
	}
}
