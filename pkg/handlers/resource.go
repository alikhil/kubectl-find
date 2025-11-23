package handlers

import (
	"context"
	"regexp"
	"time"

	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/itchyny/gojq"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s_types "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type Resource struct {
	schema.GroupVersionResource
	schema.GroupVersionKind
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

//nolint:gochecknoglobals
var ServiceType = schema.GroupVersionResource{
	Resource: "services",
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

const (
	UnknownStr = "<unknown>"
	NoneStr    = "<none>"
)

type HandlerOptions struct {
	clientSet      kubernetes.Interface
	executorGetter ExecutorGetter
	dynamic        dynamic.Interface
	allNamespaces  bool
	restarted      bool
	withImages     bool
	labels         []string
	nodeLabels     []string
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

func (o HandlerOptions) WithLabels(withLabels []string) HandlerOptions {
	o.labels = withLabels
	return o
}

func (o HandlerOptions) WithNodeLabels(withNodeLabels []string) HandlerOptions {
	o.nodeLabels = withNodeLabels
	return o
}

func GetResourceHandler(resource Resource, opts HandlerOptions) (ResourceHandler, error) {
	switch resource.GroupVersionResource {
	case PodType:
		return &PodHandler{
			clientSet: opts.clientSet,
			printer: printers.NewTablePrinter(printers.TablePrinterOptions{
				ShowNamespace:     opts.allNamespaces,
				AdditionalColumns: GetColumnsFor(opts, resource),
				LabelColumns:      GetLabelColumns(opts, resource.GroupVersionResource),
			}),
			executorGetter: opts.executorGetter,
		}, nil
	default:

		return NewUniversalHandler(UniversalHandlerOptions{
			Client: opts.dynamic,
			Printer: printers.NewTablePrinter(printers.TablePrinterOptions{
				ShowNamespace:     resource.IsNamespaced && opts.allNamespaces,
				AdditionalColumns: GetColumnsFor(opts, resource),
				LabelColumns:      GetLabelColumns(opts, resource.GroupVersionResource),
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
	SkipConfirm   bool        // skip confirmation prompt before performing actions
	ResourceType  Resource    // type of resource being handled
	JQQuery       *gojq.Query // field selector to filter resources
	ShowLabels    []string    // list of labels to show in output
	NaturalSort   bool        // sort resource names in natural order

	// Pod related options
	PodStatus      v1.PodPhase // only for pods, e.g. "Running", "Pending", etc.
	Patch          string
	PatchStrategy  k8s_types.PatchType // type of patch to apply, e.g. "json", "merge", etc.
	Exec           string              // command to execute on pods
	NodeNameRegex  *regexp.Regexp      // filter pods by node name, only applicable for pod resources
	Restarted      bool                // only for pods, find pods that have been restarted at least once
	ImageRegex     *regexp.Regexp      // filter pods by container image, only applicable for pod resources
	ShowNodeLabels []string            // list of node labels to show, only applicable for pod resources

	Streams *genericclioptions.IOStreams
}

// ResourceHandler is an interface that represents a generic resource handler.
type ResourceHandler interface {
	IsExecutable() bool
	HandleAction(ctx context.Context, options ActionOptions) error
}
