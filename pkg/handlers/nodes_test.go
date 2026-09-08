package handlers

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
