package sortby

import (
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type UnstructuredSlice []unstructured.Unstructured

func (p UnstructuredSlice) Len() int { return len(p) }
func (p UnstructuredSlice) Less(i, j int) bool {
	return Less(p[i].GetName(), p[j].GetName())
}
func (p UnstructuredSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

type PodSlice []*v1.Pod

func (p PodSlice) Len() int { return len(p) }
func (p PodSlice) Less(i, j int) bool {
	return Less(p[i].GetName(), p[j].GetName())
}
func (p PodSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
