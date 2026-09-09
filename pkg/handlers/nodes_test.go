package handlers

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodeHandlerMatchesNegatedFilters(t *testing.T) {
	handler := &NodeHandler{}
	node := v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "worker-draining",
		Labels: map[string]string{"environment": "production"},
	}}

	assert.False(t, handler.matches(node, &ActionOptions{
		ExcludedNameRegex: regexp.MustCompile("draining$"),
	}))
	assert.False(t, handler.matches(node, &ActionOptions{
		ExcludedLabelSelector: labels.SelectorFromSet(labels.Set{"environment": "production"}),
	}))
	assert.True(t, handler.matches(node, &ActionOptions{
		ExcludedNameRegex: regexp.MustCompile("control-plane$"),
	}))
}

func TestNodeHandlerListIncludesKubeletVersion(t *testing.T) {
	t.Parallel()

	clientSet := fake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{KubeletVersion: "v1.32.3"},
		},
	})
	handler, err := GetResourceHandler(
		Resource{GroupVersionResource: NodeType},
		NewHandlerOptions().WithClientSet(clientSet),
	)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	err = handler.HandleAction(context.Background(), ActionOptions{
		Action: ActionList,
		Streams: &genericclioptions.IOStreams{
			Out: output,
		},
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "VERSION")
	require.Contains(t, output.String(), "v1.32.3")
}
