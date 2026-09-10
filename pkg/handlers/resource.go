package handlers

import (
	"context"
	"regexp"
	"time"

	"github.com/itchyny/gojq"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s_types "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	appsGroup     = "apps"
	nodesResource = "nodes"
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

//nolint:gochecknoglobals
var DeploymentType = schema.GroupVersionResource{
	Resource: "deployments",
	Group:    appsGroup,
	Version:  "v1",
}

//nolint:gochecknoglobals
var StatefulSetType = schema.GroupVersionResource{
	Resource: "statefulsets",
	Group:    appsGroup,
	Version:  "v1",
}

//nolint:gochecknoglobals
var ReplicaSetType = schema.GroupVersionResource{
	Resource: "replicasets",
	Group:    appsGroup,
	Version:  "v1",
}

//nolint:gochecknoglobals
var DaemonSetType = schema.GroupVersionResource{
	Resource: "daemonsets",
	Group:    appsGroup,
	Version:  "v1",
}

//nolint:gochecknoglobals
var NodeType = schema.GroupVersionResource{
	Resource: nodesResource,
	Group:    "",
	Version:  "v1",
}

//nolint:gochecknoglobals
var ApplicationType = schema.GroupVersionResource{
	Resource: "applications",
	Group:    "argoproj.io",
	Version:  "v1alpha1",
}

type Action int

const (
	ActionList Action = iota
	ActionDelete
	ActionPatch
	ActionExec
	ActionAnnotate
	ActionCordon
	ActionUncordon
	ActionDrain
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
	case ActionAnnotate:
		return "annotate"
	case ActionCordon:
		return "cordon"
	case ActionUncordon:
		return "uncordon"
	case ActionDrain:
		return "drain"
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
	annotations    []string
	output         string
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

func (o HandlerOptions) WithAnnotations(withAnnotations []string) HandlerOptions {
	o.annotations = withAnnotations
	return o
}

func (o HandlerOptions) WithOutput(output string) HandlerOptions {
	o.output = output
	return o
}

func GetResourceHandler(resource Resource, opts HandlerOptions) (ResourceHandler, error) {
	printer, err := newBatchPrinter(opts, resource)
	if err != nil {
		return nil, err
	}

	switch resource.GroupVersionResource {
	case PodType:
		return &PodHandler{
			clientSet:      opts.clientSet,
			printer:        printer,
			executorGetter: opts.executorGetter,
		}, nil
	case NodeType:
		return &NodeHandler{
			clientSet: opts.clientSet,
			printer:   printer,
		}, nil
	default:
		return NewUniversalHandler(UniversalHandlerOptions{
			Client:          opts.dynamic,
			Printer:         printer,
			Resource:        resource,
			ResourceMatcher: getResourceMatcher(resource),
		}), nil
	}
}

type ActionOptions struct {
	Namespace             string
	ExcludedNamespace     string
	LabelSelector         string
	ExcludedLabelSelector labels.Selector
	Action                Action
	NameRegex             *regexp.Regexp
	ExcludedNameRegex     *regexp.Regexp
	MinAge                time.Duration
	MaxAge                time.Duration
	SkipConfirm           bool        // skip confirmation prompt before performing actions
	Force                 bool        // immediately remove resources from API and bypass graceful deletion (only for delete action)
	ResourceType          Resource    // type of resource being handled
	JQQuery               *gojq.Query // field selector to filter resources
	ExcludedJQQuery       *gojq.Query
	ShowLabels            []string // list of labels to show in output
	ShowAnnotations       []string // list of annotations to show in output
	NaturalSort           bool     // sort resource names in natural order

	// Annotate action options
	Annotate AnnotateConfig // parsed annotation additions and removals

	// Pod related options
	PodStatus             v1.PodPhase // only for pods, e.g. "Running", "Pending", etc.
	ExcludedPodStatus     v1.PodPhase
	Patch                 string
	PatchStrategy         k8s_types.PatchType // type of patch to apply, e.g. "json", "merge", etc.
	Exec                  string              // command to execute on pods
	NodeNameRegex         *regexp.Regexp      // filter pods by node name, only applicable for pod resources
	ExcludedNodeNameRegex *regexp.Regexp
	Restarted             bool // only for pods, find pods that have been restarted at least once
	ExcludeRestarted      bool
	ImageRegex            *regexp.Regexp // filter pods by container image, only applicable for pod resources
	ExcludedImageRegex    *regexp.Regexp
	Controller            *schema.GroupKind // direct controller owner type for pods
	ExcludedController    *schema.GroupKind
	ShowNodeLabels        []string // list of node labels to show, only applicable for pod resources

	// Node related options
	NodeConditions []NodeCondition // filter nodes by conditions, only applicable for node resources

	// Node drain options. These mirror the corresponding kubectl drain flags.
	DrainIgnoreDaemonSets      bool
	DrainDeleteEmptyDirData    bool
	DrainGracePeriodSeconds    int
	DrainTimeout               time.Duration
	DrainPodSelector           string
	DrainDisableEviction       bool
	DrainSkipWaitDeleteTimeout int
	DrainChunkSize             int64

	Streams *genericclioptions.IOStreams
}

// NodeCondition represents a node condition filter with a type and expected status.
type NodeCondition struct {
	Type   string
	Status string
}

// ResourceHandler is an interface that represents a generic resource handler.
type ResourceHandler interface {
	IsExecutable() bool
	HandleAction(ctx context.Context, options ActionOptions) error
}

// getResourceMatcher returns a ResourceMatcher for the given resource type,
// or nil if no resource-specific matching is needed.
func getResourceMatcher(resource Resource) ResourceMatcher {
	switch resource.GroupVersionResource {
	case NodeType:
		return NodeConditionMatches
	default:
		return nil
	}
}
