package handlers

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/itchyny/gojq"
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

func TestRewriteDrainFlagGuidance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "emptyDir data",
			input: "cannot delete Pods with local storage (use --delete-emptydir-data to override)",
			want:  "cannot delete Pods with local storage (use --drain-delete-emptydir-data to override)",
		},
		{
			name:  "daemonset-managed pods",
			input: "cannot delete DaemonSet-managed Pods (use --ignore-daemonsets to ignore)",
			want:  "cannot delete DaemonSet-managed Pods (use --drain-ignore-daemonsets to ignore)",
		},
		{
			name:  "both flags",
			input: "use --ignore-daemonsets and --delete-emptydir-data",
			want:  "use --drain-ignore-daemonsets and --drain-delete-emptydir-data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cause := errors.New(tt.input)
			err := rewriteDrainFlagGuidance(cause)
			require.EqualError(t, err, tt.want)
			require.ErrorIs(t, err, cause)
		})
	}
}

func TestNodeHandlerUsesKubectlDrainEvictionRetryInterval(t *testing.T) {
	t.Parallel()

	handler := &NodeHandler{clientSet: fake.NewSimpleClientset()}
	helper := handler.newDrainHelper(context.Background(), ActionOptions{
		Streams: &genericclioptions.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
	})

	require.Equal(t, 5*time.Second, helper.EvictErrorRetryDelay)
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

func TestNodeHandlerAnnotatesMatchingNodes(t *testing.T) {
	t.Parallel()

	query, err := gojq.Parse(`.status.nodeInfo.kubeletVersion | select(test("^v1\\.30\\."))`)
	require.NoError(t, err)
	clientSet := fake.NewSimpleClientset(
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "matching-node",
				Labels: map[string]string{"pool": "batch"},
			},
			Status: v1.NodeStatus{NodeInfo: v1.NodeSystemInfo{KubeletVersion: "v1.30.9"}},
		},
		&v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "other-version",
				Labels: map[string]string{"pool": "batch"},
			},
			Status: v1.NodeStatus{NodeInfo: v1.NodeSystemInfo{KubeletVersion: "v1.31.4"}},
		},
	)
	handler, err := GetResourceHandler(
		Resource{GroupVersionResource: NodeType},
		NewHandlerOptions().WithClientSet(clientSet),
	)
	require.NoError(t, err)

	err = handler.HandleAction(t.Context(), ActionOptions{
		Action:        ActionAnnotate,
		LabelSelector: "pool=batch",
		JQQuery:       query,
		SkipConfirm:   true,
		Annotate:      AnnotateConfig{Add: map[string]string{"maintenance.example/enabled": "true"}},
		Streams:       &genericclioptions.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
	})
	require.NoError(t, err)

	matching, err := clientSet.CoreV1().Nodes().Get(t.Context(), "matching-node", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "true", matching.Annotations["maintenance.example/enabled"])

	nonMatching, err := clientSet.CoreV1().Nodes().Get(t.Context(), "other-version", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, nonMatching.Annotations, "maintenance.example/enabled")
}

func TestHandlerActionCompatibilityMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		handler   ResourceHandler
		supported []Action
	}{
		{
			name:      "pods",
			handler:   &PodHandler{},
			supported: []Action{ActionList, ActionDelete, ActionPatch, ActionExec, ActionAnnotate, ActionEvict},
		},
		{
			name:      "nodes",
			handler:   &NodeHandler{},
			supported: []Action{ActionList, ActionAnnotate, ActionCordon, ActionUncordon, ActionDrain},
		},
		{
			name: "universal resources",
			handler: NewUniversalHandler(UniversalHandlerOptions{
				Resource: Resource{GroupVersionResource: ServiceType},
			}),
			supported: []Action{ActionList, ActionDelete, ActionPatch, ActionAnnotate},
		},
		{
			name: "restartable workloads",
			handler: NewUniversalHandler(UniversalHandlerOptions{
				Resource: Resource{GroupVersionResource: DeploymentType},
			}),
			supported: []Action{ActionList, ActionDelete, ActionPatch, ActionAnnotate, ActionRestart},
		},
	}

	actions := []Action{
		ActionList,
		ActionDelete,
		ActionPatch,
		ActionExec,
		ActionAnnotate,
		ActionCordon,
		ActionUncordon,
		ActionDrain,
		ActionRestart,
		ActionEvict,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, action := range actions {
				expected := false
				for _, supported := range tt.supported {
					if action == supported {
						expected = true
						break
					}
				}
				require.Equalf(t, expected, tt.handler.SupportsAction(action), "action %s", action)
			}
		})
	}
}
