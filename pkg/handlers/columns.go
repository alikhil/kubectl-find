package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/alikhil/kubectl-find/pkg/printers"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/jsonpath"
)

const defaultReplicaCount = int32(1)

func labelToColumnHeader(label string) string {
	parts := strings.Split(label, "/")
	//nolint:mnd // common case to have prefix with slash
	if len(parts) == 2 {
		return strings.ToUpper(parts[1])
	}
	return strings.ToUpper(label)
}

func getColumnsForPods(opts HandlerOptions) []printers.Column {
	columns := []printers.Column{
		{
			Header: "READY",
			Value: func(obj unstructured.Unstructured) string {
				pod, err := toPod(obj)
				if err != nil {
					return UnknownStr
				}
				totalContainers := len(pod.Spec.Containers)
				readyContainers := 0
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Ready {
						readyContainers++
					}
				}
				return fmt.Sprintf("%d/%d", readyContainers, totalContainers)
			},
		},
		{
			Header: "STATUS",
			Value: func(obj unstructured.Unstructured) string {
				if status, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
					return status
				}
				return UnknownStr
			},
		},
		{
			Header: "RESTARTS",
			Value: func(obj unstructured.Unstructured) string {
				pod, err := toPod(obj)
				if err != nil {
					return UnknownStr
				}
				totalRestarts := 0
				for _, cs := range pod.Status.ContainerStatuses {
					totalRestarts += int(cs.RestartCount)
				}
				return fmt.Sprintf("%d", totalRestarts)
			},
		},
	}
	if opts.withImages {
		columns = append(columns, printers.Column{
			Header: "IMAGES",
			Value: func(obj unstructured.Unstructured) string {
				pod, err := toPod(obj)
				if err != nil {
					return UnknownStr
				}
				var images []string
				for _, container := range pod.Spec.Containers {
					images = append(images, container.Image)
				}
				return strings.Join(images, ", ")
			},
		})
	}
	return columns
}

func getColumnsForServices(_ HandlerOptions) []printers.Column {
	columns := []printers.Column{
		{
			Header: "TYPE",
			Value: func(obj unstructured.Unstructured) string {
				if svcType, found, _ := unstructured.NestedString(obj.Object, "spec", "type"); found {
					return svcType
				}
				return UnknownStr
			},
		},
		{
			Header: "CLUSTER-IP",
			Value: func(obj unstructured.Unstructured) string {
				if clusterIP, found, _ := unstructured.NestedString(obj.Object, "spec", "clusterIP"); found {
					return clusterIP
				}
				return NoneStr
			},
		},
		{
			Header: "EXTERNAL-IP",
			Value: func(obj unstructured.Unstructured) string {
				if ingress, found, _ := unstructured.NestedSlice(obj.Object, "status", "loadBalancer", "ingress"); found &&
					len(ingress) > 0 {
					var ips []string
					for _, entry := range ingress {
						if entryMap, ok := entry.(map[string]interface{}); ok {
							if ip, ipFound := entryMap["ip"].(string); ipFound {
								ips = append(ips, ip)
							}
						}
					}
					return strings.Join(ips, ", ")
				}
				return NoneStr
			},
		},
		{
			Header: "PORT(S)",
			Value: func(obj unstructured.Unstructured) string {
				if ports, found, _ := unstructured.NestedSlice(obj.Object, "spec", "ports"); found {
					var portStrs []string
					for _, port := range ports {
						if portMap, ok := port.(map[string]interface{}); ok {
							portNum := portMap["port"]
							nodePort := portMap["nodePort"]
							protocol := portMap["protocol"]
							var port string
							if nodePort != nil {
								port = fmt.Sprintf("%v:%v/%v", portNum, nodePort, protocol)
							} else {
								port = fmt.Sprintf("%v/%v", portNum, protocol)
							}
							portStrs = append(portStrs, port)
						}
					}
					//nolint:mnd // limit output length
					if len(portStrs) > 10 {
						// TODO: detect consecutive ports, show as range
						return strings.Join(portStrs[:10], ",") + " ..."
					}
					return strings.Join(portStrs, ",")
				}
				return NoneStr
			},
		},
	}
	return columns
}

func getReplicaCountOrDefault(replicas *int32) int32 {
	if replicas == nil {
		return defaultReplicaCount
	}
	return *replicas
}

func toPod(obj unstructured.Unstructured) (*v1.Pod, error) {
	pod := &v1.Pod{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

func toDeployment(obj unstructured.Unstructured) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, deployment); err != nil {
		return nil, err
	}
	return deployment, nil
}

func toStatefulSet(obj unstructured.Unstructured) (*appsv1.StatefulSet, error) {
	statefulSet := &appsv1.StatefulSet{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, statefulSet); err != nil {
		return nil, err
	}
	return statefulSet, nil
}

func toReplicaSet(obj unstructured.Unstructured) (*appsv1.ReplicaSet, error) {
	replicaSet := &appsv1.ReplicaSet{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, replicaSet); err != nil {
		return nil, err
	}
	return replicaSet, nil
}

func toDaemonSet(obj unstructured.Unstructured) (*appsv1.DaemonSet, error) {
	daemonSet := &appsv1.DaemonSet{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, daemonSet); err != nil {
		return nil, err
	}
	return daemonSet, nil
}

func getColumnsForDeployments() []printers.Column {
	return []printers.Column{
		{
			Header: "READY",
			Value: func(obj unstructured.Unstructured) string {
				deployment, err := toDeployment(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf(
					"%d/%d",
					deployment.Status.ReadyReplicas,
					getReplicaCountOrDefault(deployment.Spec.Replicas),
				)
			},
		},
		{
			Header: "UP-TO-DATE",
			Value: func(obj unstructured.Unstructured) string {
				deployment, err := toDeployment(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", deployment.Status.UpdatedReplicas)
			},
		},
		{
			Header: "AVAILABLE",
			Value: func(obj unstructured.Unstructured) string {
				deployment, err := toDeployment(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", deployment.Status.AvailableReplicas)
			},
		},
	}
}

func getColumnsForStatefulSets() []printers.Column {
	return []printers.Column{
		{
			Header: "READY",
			Value: func(obj unstructured.Unstructured) string {
				statefulSet, err := toStatefulSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf(
					"%d/%d",
					statefulSet.Status.ReadyReplicas,
					getReplicaCountOrDefault(statefulSet.Spec.Replicas),
				)
			},
		},
	}
}

func getColumnsForReplicaSets() []printers.Column {
	return []printers.Column{
		{
			Header: "DESIRED",
			Value: func(obj unstructured.Unstructured) string {
				replicaSet, err := toReplicaSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", getReplicaCountOrDefault(replicaSet.Spec.Replicas))
			},
		},
		{
			Header: "CURRENT",
			Value: func(obj unstructured.Unstructured) string {
				replicaSet, err := toReplicaSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", replicaSet.Status.Replicas)
			},
		},
		{
			Header: "READY",
			Value: func(obj unstructured.Unstructured) string {
				replicaSet, err := toReplicaSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", replicaSet.Status.ReadyReplicas)
			},
		},
	}
}

func getColumnsForDaemonSets() []printers.Column {
	return []printers.Column{
		{
			Header: "DESIRED",
			Value: func(obj unstructured.Unstructured) string {
				daemonSet, err := toDaemonSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", daemonSet.Status.DesiredNumberScheduled)
			},
		},
		{
			Header: "CURRENT",
			Value: func(obj unstructured.Unstructured) string {
				daemonSet, err := toDaemonSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", daemonSet.Status.CurrentNumberScheduled)
			},
		},
		{
			Header: "READY",
			Value: func(obj unstructured.Unstructured) string {
				daemonSet, err := toDaemonSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", daemonSet.Status.NumberReady)
			},
		},
		{
			Header: "UP-TO-DATE",
			Value: func(obj unstructured.Unstructured) string {
				daemonSet, err := toDaemonSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", daemonSet.Status.UpdatedNumberScheduled)
			},
		},
		{
			Header: "AVAILABLE",
			Value: func(obj unstructured.Unstructured) string {
				daemonSet, err := toDaemonSet(obj)
				if err != nil {
					return UnknownStr
				}
				return fmt.Sprintf("%d", daemonSet.Status.NumberAvailable)
			},
		},
	}
}

func GetColumnsFor(opts HandlerOptions, resourceType Resource) []printers.Column {
	switch resourceType.GroupVersionResource {
	case PodType:
		return getColumnsForPods(opts)
	case ServiceType:
		return getColumnsForServices(opts)
	case DeploymentType:
		return getColumnsForDeployments()
	case StatefulSetType:
		return getColumnsForStatefulSets()
	case ReplicaSetType:
		return getColumnsForReplicaSets()
	case DaemonSetType:
		return getColumnsForDaemonSets()
	default:

		if !isBuiltin(scheme.Scheme, resourceType.GroupVersionKind) {
			// Check if this is a CRD and try to get additionalPrinterColumns
			if columns := getColumnsFromCRD(opts, resourceType.GroupVersionResource); columns != nil {
				return columns
			}
		}
		return nil
	}
}

func isBuiltin(sh *runtime.Scheme, resourceType schema.GroupVersionKind) bool {
	knownTypes := sh.KnownTypes(resourceType.GroupVersion())
	_, found := knownTypes[resourceType.Kind]
	return found
}

func getColumnsFromCRD(opts HandlerOptions, resourceType schema.GroupVersionResource) []printers.Column {
	// CRDs are in apiextensions.k8s.io/v1
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	// Get the CRD definition
	// CRD name format: <resource>.<group>
	crdName := fmt.Sprintf("%s.%s", resourceType.Resource, resourceType.Group)
	crd, err := opts.dynamic.Resource(crdGVR).Get(context.Background(), crdName, metav1.GetOptions{})
	if err != nil {
		// Not a CRD or CRD not found
		return nil
	}
	// Cast to CustomResourceDefinition for type-safe access
	crdTyped := &apiextensionsv1.CustomResourceDefinition{}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(crd.Object, crdTyped, true); err != nil {
		return nil
	}

	// Find the version matching our resource
	for _, version := range crdTyped.Spec.Versions {
		if version.Name != resourceType.Version {
			continue
		}

		// Get additionalPrinterColumns for this version
		if len(version.AdditionalPrinterColumns) == 0 {
			continue
		}

		return convertCRDColumnsToTableColumns(version.AdditionalPrinterColumns)
	}

	return nil
}

func convertCRDColumnsToTableColumns(crdColumns []apiextensionsv1.CustomResourceColumnDefinition) []printers.Column {
	var columns []printers.Column

	for _, col := range crdColumns {
		name := col.Name
		jsonPathStr := col.JSONPath

		if col.Priority > 0 {
			// skipping low priority columns for now
			// todo: add option to include them possible with -o wide flag
			continue
		}

		if col.Name == "Age" {
			// Age column is handled separately by printers, skip it here
			continue
		}

		// Parse the JSONPath expression
		jp := jsonpath.New(name)
		if err := jp.Parse(fmt.Sprintf("{%s}", jsonPathStr)); err != nil {
			// If parsing fails, skip this column
			continue
		}

		columns = append(columns, printers.Column{
			Header: strings.ToUpper(name),
			Value: func(obj unstructured.Unstructured) string {
				return extractValueFromJSONPath(obj, jp)
			},
		})
	}

	return columns
}

func extractValueFromJSONPath(obj unstructured.Unstructured, jp *jsonpath.JSONPath) string {
	// Execute the JSONPath query
	results, err := jp.FindResults(obj.UnstructuredContent())
	if err != nil || len(results) == 0 || len(results[0]) == 0 {
		return NoneStr
	}

	// Get the first result value
	val := results[0][0].Interface()

	// Special handling for common types
	switch v := val.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int64, int32, int, float64, float32:
		return fmt.Sprintf("%v", v)
	case nil:
		return NoneStr
	default:
		// For complex types or timestamps, format as string
		return fmt.Sprintf("%v", v)
	}
}

func getCacheFunc(client kubernetes.Interface) func(nodeName, labelKey string) string {
	nodeCache := make(map[string]*v1.Node)
	labelCache := make(map[string]map[string]string)
	return func(nodeName, labelKey string) string {
		if labels, labelsFound := labelCache[nodeName]; labelsFound {
			if labelValue, found := labels[labelKey]; found {
				return labelValue
			}

			if labelValue, found := nodeCache[nodeName].Labels[labelKey]; found {
				labelCache[nodeName][labelKey] = labelValue
				return labelValue
			}

			labelCache[nodeName][labelKey] = NoneStr
			return NoneStr
		}
		if _, ok := nodeCache[nodeName]; !ok {
			node, err := client.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
			if err != nil {
				return UnknownStr
			}
			nodeCache[nodeName] = node
		}
		if labelValue, found := nodeCache[nodeName].Labels[labelKey]; found {
			if _, ok := labelCache[nodeName]; !ok {
				labelCache[nodeName] = make(map[string]string)
			}
			labelCache[nodeName][labelKey] = labelValue
			return labelValue
		}
		return NoneStr
	}
}

func GetLabelColumns(opts HandlerOptions, res schema.GroupVersionResource) []printers.Column {
	columns := []printers.Column{}
	if len(opts.labels) > 0 {
		for _, labelKey := range opts.labels {
			key := labelKey // capture loop variable
			columns = append(columns, printers.Column{
				Header: labelToColumnHeader(key),
				Value: func(obj unstructured.Unstructured) string {
					if labelValue, found, _ := unstructured.NestedString(obj.Object, "metadata", "labels", key); found {
						return labelValue
					}
					return NoneStr
				},
			})
		}
	}
	if res == PodType && len(opts.nodeLabels) > 0 {
		columns = append(columns, printers.Column{
			Header: "NODE",
			Value: func(obj unstructured.Unstructured) string {
				if nodeName, found, _ := unstructured.NestedString(obj.Object, "spec", "nodeName"); found {
					return nodeName
				}
				return UnknownStr
			},
		})

		for _, labelKey := range opts.nodeLabels {
			key := labelKey // capture loop variable
			cacheFunc := getCacheFunc(opts.clientSet)
			columns = append(columns, printers.Column{
				Header: labelToColumnHeader(key),
				Value: func(obj unstructured.Unstructured) string {
					nodeName, found, _ := unstructured.NestedString(obj.Object, "spec", "nodeName")
					if !found {
						return UnknownStr
					}
					return cacheFunc(nodeName, key)
				},
			})
		}
	}
	return columns
}

func GetAnnotationColumns(opts HandlerOptions) []printers.Column {
	columns := []printers.Column{}
	if len(opts.annotations) > 0 {
		for _, annotationKey := range opts.annotations {
			key := annotationKey // capture loop variable
			columns = append(columns, printers.Column{
				Header: labelToColumnHeader(key),
				Value: func(obj unstructured.Unstructured) string {
					if annotationValue, found, _ := unstructured.NestedString(obj.Object, "metadata", "annotations", key); found {
						return annotationValue
					}
					return NoneStr
				},
			})
		}
	}
	return columns
}
