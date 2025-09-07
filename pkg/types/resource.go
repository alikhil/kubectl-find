package types

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alikhil/kubectl-find/pkg/printers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s_types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type Resource struct {
	schema.GroupVersionResource
	PluralName   string
	SingularName string
	IsNamespaced bool
}

//nolint:gochecknoglobals
var PodType = schema.GroupVersionResource{
	Resource: "pods",
	Group:    "",
	Version:  "v1",
}

type Action int

const (
	ActionList Action = iota
	ActionDelete
	ActionPatch
	ActionExec
)

func (a Action) String() string {
	switch a {
	case ActionList:
		return "list"
	case ActionDelete:
		return "delete"
	case ActionPatch:
		return "patch"
	case ActionExec:
		return "exec"
	default:
		return "Unknown"
	}
}

type HandlerOptions struct {
	clientSet      kubernetes.Interface
	executorGetter ExecutorGetter
	dynamic        dynamic.Interface
	allNamespaces  bool
	restarted      bool
	withImages     bool
}

func NewHandlerOptions() HandlerOptions {
	return HandlerOptions{}
}

func (o HandlerOptions) WithClientSet(clientSet kubernetes.Interface) HandlerOptions {
	o.clientSet = clientSet
	return o
}

func (o HandlerOptions) WithExecutorGetter(executorGetter ExecutorGetter) HandlerOptions {
	o.executorGetter = executorGetter
	return o
}

func (o HandlerOptions) WithNamespaced(allNamespaces bool) HandlerOptions {
	o.allNamespaces = allNamespaces
	return o
}

func (o HandlerOptions) WithRestarted(restarted bool) HandlerOptions {
	o.restarted = restarted
	return o
}

func (o HandlerOptions) WithImages(withImages bool) HandlerOptions {
	o.withImages = withImages
	return o
}

func (o HandlerOptions) WithDynamic(dynamic dynamic.Interface) HandlerOptions {
	o.dynamic = dynamic
	return o
}

func GetResourceHandler(resource Resource, opts HandlerOptions) (ResourceHandler, error) {
	switch resource.GroupVersionResource {
	case PodType:
		columns := []printers.Column{
			{
				Header: "STATUS",
				Value: func(obj unstructured.Unstructured) string {
					if status, found, _ := unstructured.NestedString(obj.Object, "status", "phase"); found {
						return status
					}
					return "<unknown>"
				},
			},
		}
		if opts.restarted {
			columns = append(columns, printers.Column{
				Header: "RESTARTS",
				Value: func(obj unstructured.Unstructured) string {
					pod := &v1.Pod{}
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, pod); err != nil {
						return "<unknown>"
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
						return "<unknown>"
					}
					var images []string
					for _, container := range pod.Spec.Containers {
						images = append(images, container.Image)
					}
					return strings.Join(images, ", ")
				},
			})
		}
		return &PodHandler{
			clientSet: opts.clientSet,
			printer: printers.NewTablePrinter(printers.TablePrinterOptions{
				ShowNamespace:     opts.allNamespaces,
				AdditionalColumns: columns,
			}),
			executorGetter: opts.executorGetter,
		}, nil
	default:

		return NewUniversalHandler(UniversalHandlerOptions{
			Client: opts.dynamic,
			Printer: printers.NewTablePrinter(printers.TablePrinterOptions{
				ShowNamespace: resource.IsNamespaced && opts.allNamespaces,
			}),
			Resource: resource,
		}), nil
	}
}

type ActionOptions struct {
	Namespace     string
	LabelSelector string
	Action        Action
	NameRegex     *regexp.Regexp
	MinAge        time.Duration
	MaxAge        time.Duration
	SkipConfirm   bool     // skip confirmation prompt before performing actions
	ResourceType  Resource // type of resource being handled

	// Pod related options
	PodStatus     v1.PodPhase // only for pods, e.g. "Running", "Pending", etc.
	Patch         string
	PatchStrategy k8s_types.PatchType // type of patch to apply, e.g. "json", "merge", etc.
	Exec          string              // command to execute on pods
	NodeNameRegex *regexp.Regexp      // filter pods by node name, only applicable for pod resources
	Restarted     bool                // only for pods, find pods that have been restarted at least once
	ImageRegex    *regexp.Regexp      // filter pods by container image, only applicable for pod resources

	Streams *genericclioptions.IOStreams
}

// ResourceHandler is an interface that represents a generic resource handler.
type ResourceHandler interface {
	IsExecutable() bool
	HandleAction(ctx context.Context, options ActionOptions) error
}
