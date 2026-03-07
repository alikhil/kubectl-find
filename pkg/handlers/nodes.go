package handlers

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NodeConditionMatcher returns a ResourceMatcher that filters nodes by conditions.
// All specified conditions must match (AND logic). Comparison is case-insensitive.
func NodeConditionMatcher() ResourceMatcher {
	return func(resource unstructured.Unstructured, options *ActionOptions) bool {
		if len(options.NodeConditions) == 0 {
			return true
		}
		return nodeConditionMatches(resource, options.NodeConditions)
	}
}

func nodeConditionMatches(
	resource unstructured.Unstructured,
	conditions []NodeCondition,
) bool {
	conditionsRaw, found, _ := unstructured.NestedSlice(resource.Object, "status", "conditions")
	if !found {
		return false
	}

	conditionMap := make(map[string]string, len(conditionsRaw))
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

	for _, nc := range conditions {
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
