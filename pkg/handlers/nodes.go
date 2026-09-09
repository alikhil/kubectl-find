package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alikhil/kubectl-find/pkg"
	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/alikhil/kubectl-find/pkg/prompts"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/drain"
)

// NodeHandler handles node-specific actions while applying the same inclusive
// and negated filters as the universal resource handler.
type NodeHandler struct {
	clientSet kubernetes.Interface
	printer   printers.BatchPrinter
}

func (h *NodeHandler) IsExecutable() bool { return false }

func (h *NodeHandler) HandleAction(ctx context.Context, options ActionOptions) error {
	list, listErr := h.clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: options.LabelSelector})
	if listErr != nil {
		return fmt.Errorf("failed to list nodes: %w", listErr)
	}

	nodes := make([]v1.Node, 0, len(list.Items))
	for _, node := range list.Items {
		if h.matches(node, &options) {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nil
	}
	if options.NaturalSort {
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	}

	if options.Action == ActionList {
		objects := make([]unstructured.Unstructured, 0, len(nodes))
		for _, node := range nodes {
			object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&node)
			if err != nil {
				return fmt.Errorf("failed to convert node %s to unstructured: %w", node.Name, err)
			}
			objects = append(objects, unstructured.Unstructured{Object: object})
		}
		return h.printer.PrintObjects(objects, options.Streams.Out)
	}
	if options.Action != ActionCordon && options.Action != ActionUncordon && options.Action != ActionDrain {
		return fmt.Errorf("unsupported action: %s", options.Action)
	}
	if !options.SkipConfirm {
		fmt.Fprintf(options.Streams.ErrOut, "The following nodes will be %sed:\n", options.Action)
		for _, node := range nodes {
			fmt.Fprintf(options.Streams.ErrOut, "- %s\n", node.Name)
		}
		if !prompts.AskForConfirmation(options.Streams) {
			fmt.Fprintf(options.Streams.ErrOut, "%s cancelled.\n", options.Action.String())
			return nil
		}
	}

	drainer := &drain.Helper{
		Ctx:                             ctx,
		Client:                          h.clientSet,
		Force:                           options.Force,
		IgnoreAllDaemonSets:             options.DrainIgnoreDaemonSets,
		DeleteEmptyDirData:              options.DrainDeleteEmptyDirData,
		GracePeriodSeconds:              options.DrainGracePeriodSeconds,
		Timeout:                         options.DrainTimeout,
		PodSelector:                     options.DrainPodSelector,
		DisableEviction:                 options.DrainDisableEviction,
		SkipWaitForDeleteTimeoutSeconds: options.DrainSkipWaitDeleteTimeout,
		ChunkSize:                       options.DrainChunkSize,
		Out:                             options.Streams.Out,
		ErrOut:                          options.Streams.ErrOut,
	}
	for i := range nodes {
		node := &nodes[i]
		if options.Action == ActionCordon || options.Action == ActionDrain {
			if err := drain.RunCordonOrUncordon(drainer, node, true); err != nil {
				return fmt.Errorf("failed to cordon node %s: %w", node.Name, err)
			}
		}
		if options.Action == ActionUncordon {
			if err := drain.RunCordonOrUncordon(drainer, node, false); err != nil {
				return fmt.Errorf("failed to uncordon node %s: %w", node.Name, err)
			}
			fmt.Fprintf(options.Streams.Out, "node/%s uncordoned\n", node.Name)
			continue
		}
		if options.Action == ActionCordon {
			fmt.Fprintf(options.Streams.Out, "node/%s cordoned\n", node.Name)
			continue
		}
		if err := drain.RunNodeDrain(drainer, node.Name); err != nil {
			return fmt.Errorf("failed to drain node %s: %w", node.Name, err)
		}
		fmt.Fprintf(options.Streams.Out, "node/%s drained\n", node.Name)
	}
	return nil
}

func (h *NodeHandler) matches(node v1.Node, options *ActionOptions) bool {
	if options.NameRegex != nil && !options.NameRegex.MatchString(node.Name) {
		return false
	}
	if options.ExcludedNameRegex != nil && options.ExcludedNameRegex.MatchString(node.Name) {
		return false
	}
	if options.ExcludedLabelSelector != nil && options.ExcludedLabelSelector.Matches(labels.Set(node.Labels)) {
		return false
	}
	if options.MinAge > 0 && time.Since(node.CreationTimestamp.Time) < options.MinAge {
		return false
	}
	if options.MaxAge > 0 && time.Since(node.CreationTimestamp.Time) > options.MaxAge {
		return false
	}
	object, conversionErr := runtime.DefaultUnstructuredConverter.ToUnstructured(&node)
	if conversionErr != nil {
		return false
	}
	resource := unstructured.Unstructured{Object: object}
	if !NodeConditionMatches(resource, options) {
		return false
	}
	if options.JQQuery != nil {
		matches, matchErr := pkg.MatchesWithGoJQ(resource.Object, options.JQQuery)
		if matchErr != nil || !matches {
			return false
		}
	}
	if options.ExcludedJQQuery != nil {
		matches, matchErr := pkg.MatchesWithGoJQ(resource.Object, options.ExcludedJQQuery)
		if matchErr == nil && matches {
			return false
		}
	}
	return true
}

// NodeConditionMatches is a ResourceMatcher that filters nodes by conditions.
// All specified conditions must match (AND logic). Comparison is case-insensitive.
func NodeConditionMatches(resource unstructured.Unstructured, options *ActionOptions) bool {
	if len(options.NodeConditions) == 0 {
		return true
	}

	conditionsRaw, _, _ := unstructured.NestedSlice(resource.Object, "status", "conditions")
	conditionMap := make(map[string]string, len(conditionsRaw)+1)
	for _, c := range conditionsRaw {
		cMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		cType, _ := cMap["type"].(string)
		cStatus, _ := cMap["status"].(string)
		if cType != "" {
			conditionMap[strings.ToLower(cType)] = strings.ToLower(cStatus)
		}
	}
	unschedulable, _, _ := unstructured.NestedBool(resource.Object, "spec", "unschedulable")
	if unschedulable {
		conditionMap["schedulingdisabled"] = "true"
	} else {
		conditionMap["schedulingdisabled"] = "false"
	}

	for _, nc := range options.NodeConditions {
		actual, exists := conditionMap[strings.ToLower(nc.Type)]
		if !exists {
			return false
		}
		if actual != strings.ToLower(nc.Status) {
			return false
		}
	}

	return true
}
