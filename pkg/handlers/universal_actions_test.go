package handlers

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s_types "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestUniversalHandlerActionScope(t *testing.T) {
	t.Parallel()

	actions := []struct {
		action Action
		verb   string
		check  func(*testing.T, *unstructured.Unstructured, error)
	}{
		{
			action: ActionDelete,
			verb:   "delete",
			check: func(t *testing.T, _ *unstructured.Unstructured, err error) {
				require.True(t, apierrors.IsNotFound(err), "expected object to be deleted, got %v", err)
			},
		},
		{
			action: ActionPatch,
			verb:   "patch",
			check: func(t *testing.T, object *unstructured.Unstructured, err error) {
				require.NoError(t, err)
				require.Equal(t, "true", object.GetLabels()["patched"])
			},
		},
		{
			action: ActionAnnotate,
			verb:   "patch",
			check: func(t *testing.T, object *unstructured.Unstructured, err error) {
				require.NoError(t, err)
				require.Equal(t, "true", object.GetAnnotations()["annotated"])
			},
		},
		{
			action: ActionRestart,
			verb:   "patch",
			check: func(t *testing.T, object *unstructured.Unstructured, err error) {
				require.NoError(t, err)
				restartedAt, found, nestedErr := unstructured.NestedString(
					object.Object, "spec", "template", "metadata", "annotations", "kubectl.kubernetes.io/restartedAt",
				)
				require.NoError(t, nestedErr)
				require.True(t, found)
				_, parseErr := time.Parse(time.RFC3339, restartedAt)
				require.NoError(t, parseErr)
			},
		},
	}
	scopes := []struct {
		name       string
		resource   Resource
		kind       string
		namespaces []string
		namespace  string
	}{
		{
			name: "all namespaces",
			resource: Resource{
				GroupVersionResource: DeploymentType,
				SingularName:         "deployment",
				PluralName:           "deployments",
				IsNamespaced:         true,
			},
			kind:       "Deployment",
			namespaces: []string{"default", "kube-system"},
		},
		{
			name: "explicit namespace",
			resource: Resource{
				GroupVersionResource: DeploymentType,
				SingularName:         "deployment",
				PluralName:           "deployments",
				IsNamespaced:         true,
			},
			kind:       "Deployment",
			namespaces: []string{"default", "kube-system"},
			namespace:  "kube-system",
		},
		{
			name: "cluster scoped",
			resource: Resource{
				GroupVersionResource: schema.GroupVersionResource{Version: "v1", Resource: "namespaces"},
				SingularName:         "namespace",
				PluralName:           "namespaces",
			},
			kind:       "Namespace",
			namespaces: []string{""},
			namespace:  "kube-system",
		},
	}

	for _, scope := range scopes {
		for _, action := range actions {
			// Rollout restart is only supported for workloads; the compatibility matrix covers rejections.
			if action.action == ActionRestart && !scope.resource.IsNamespaced {
				continue
			}
			t.Run(scope.name+"/"+action.action.String(), func(t *testing.T) {
				t.Parallel()

				var originals []*unstructured.Unstructured
				var objects []runtime.Object
				for _, namespace := range scope.namespaces {
					for _, name := range []string{"kube-test", "unmatched"} {
						object := &unstructured.Unstructured{}
						object.SetAPIVersion(scope.resource.GroupVersionResource.GroupVersion().String())
						object.SetKind(scope.kind)
						object.SetName(name)
						if namespace != "" {
							object.SetNamespace(namespace)
						}
						originals = append(originals, object.DeepCopy())
						objects = append(objects, object)
					}
				}
				client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
					runtime.NewScheme(),
					map[schema.GroupVersionResource]string{scope.resource.GroupVersionResource: scope.kind + "List"},
					objects...,
				)
				streams, in, _, errOut := genericiooptions.NewTestIOStreams()
				in.WriteString("y\n")
				handler := NewUniversalHandler(UniversalHandlerOptions{Client: client, Resource: scope.resource})
				err := handler.HandleAction(t.Context(), ActionOptions{
					Action:        action.action,
					Namespace:     scope.namespace,
					NameRegex:     regexp.MustCompile("^kube-test$"),
					Patch:         `{"metadata":{"labels":{"patched":"true"}}}`,
					PatchStrategy: k8s_types.MergePatchType,
					Annotate:      AnnotateConfig{Add: map[string]string{"annotated": "true"}},
					Streams:       &streams,
				})
				require.NoError(t, err)

				var mutationNamespaces []string
				for _, request := range client.Actions() {
					if request.GetVerb() == action.verb {
						mutationNamespaces = append(mutationNamespaces, request.GetNamespace())
					}
				}
				var expectedNamespaces []string
				for _, original := range originals {
					updated, getErr := client.Resource(scope.resource.GroupVersionResource).
						Namespace(original.GetNamespace()).Get(t.Context(), original.GetName(), metav1.GetOptions{})
					matches := original.GetName() == "kube-test" &&
						(!scope.resource.IsNamespaced || scope.namespace == "" || original.GetNamespace() == scope.namespace)
					if matches {
						expectedNamespaces = append(expectedNamespaces, original.GetNamespace())
						action.check(t, updated, getErr)
						if scope.resource.IsNamespaced && scope.namespace == "" {
							require.Contains(t, errOut.String(), "- kube-test in namespace "+original.GetNamespace())
						}
					} else {
						require.NoError(t, getErr)
						require.Equal(t, original.Object, updated.Object, "unmatched objects must remain unchanged")
					}
				}
				require.ElementsMatch(t, expectedNamespaces, mutationNamespaces)
			})
		}
	}
}
