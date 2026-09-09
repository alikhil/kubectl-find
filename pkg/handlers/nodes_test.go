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

func TestNodeHandlerFiltersUncordonedNodesBySchedulingDisabledCondition(t *testing.T) {
	t.Parallel()

	readyCondition := v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue}
	clientSet := fake.NewSimpleClientset(
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{readyCondition}},
		},
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-2"},
			Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{readyCondition}},
		},
	)
	handler, err := GetResourceHandler(
		Resource{GroupVersionResource: NodeType},
		NewHandlerOptions().WithClientSet(clientSet),
	)
	require.NoError(t, err)

	streams := &genericclioptions.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	err = handler.HandleAction(context.Background(), ActionOptions{
		Action:      ActionCordon,
		NameRegex:   regexp.MustCompile("^worker-2$"),
		SkipConfirm: true,
		Streams:     streams,
	})
	require.NoError(t, err)

	cordonedNode, err := clientSet.CoreV1().Nodes().Get(context.Background(), "worker-2", metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, cordonedNode.Spec.Unschedulable)

	output := &bytes.Buffer{}
	err = handler.HandleAction(context.Background(), ActionOptions{
		Action: ActionList,
		NodeConditions: []NodeCondition{
			{Type: "SchedulingDisabled", Status: "False"},
		},
		Streams: &genericclioptions.IOStreams{Out: output},
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "worker-1")
	require.NotContains(t, output.String(), "worker-2")
}
