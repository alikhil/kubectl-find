package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/alikhil/kubectl-find/pkg/printers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/duration"
)

func GetColumnsForPods(opts HandlerOptions) []printers.Column {
	columns := []printers.Column{
		{
			Header: "STATUS",
			Value: func(obj unstructured.Unstructured) string {
				if status, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
					return status
				}
				return UnknownStr
			},
		},
	}
	if opts.restarted {
		columns = append(columns, printers.Column{
			Header: "RESTARTS",
			Value: func(obj unstructured.Unstructured) string {
				pod := &v1.Pod{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, pod); err != nil {
					return UnknownStr
				}
				totalRestarts := 0
				lastRestart := time.Time{}
				for _, cs := range pod.Status.ContainerStatuses {
					totalRestarts += int(cs.RestartCount)
					if cs.RestartCount > 0 &&
						lastRestart.Before(cs.LastTerminationState.Terminated.FinishedAt.Time) {
						lastRestart = cs.LastTerminationState.Terminated.FinishedAt.Time
					}
				}
				return fmt.Sprintf(
					"%d (%s ago)",
					totalRestarts,
					duration.HumanDuration(time.Since(lastRestart)),
				)
			},
		})
	}
	if opts.withImages {
		columns = append(columns, printers.Column{
			Header: "IMAGES",
			Value: func(obj unstructured.Unstructured) string {
				pod := &v1.Pod{}
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, pod); err != nil {
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

func GetColumnsForServices(_ HandlerOptions) []printers.Column {
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

func GetColumnsFor(opts HandlerOptions, resourceType schema.GroupVersionResource) []printers.Column {
	switch resourceType {
	case PodType:
		return GetColumnsForPods(opts)
	case ServiceType:
		return GetColumnsForServices(opts)
	default:
		return []printers.Column{}
	}
}
