/*
Copyright 2025 Alik Khilazhev

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/alikhil/kubectl-find/pkg"
	"github.com/alikhil/kubectl-find/pkg/handlers"
	"github.com/alikhil/kubectl-find/pkg/printers"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/remotecommand"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

var (
	//nolint:gochecknoglobals
	findExample = `
	# find pods with names matching prefix
	%[1]s fd --name mypod-*

	# find secrets created more than 2 days ago in specified namespace
	%[1]s fd secrets --min-age 2d -n superapp

	# find all externalecrets and patch them to force sync
	%[1]s fd externalsecret -A --patch '{"metadata": {"annotations": {"force-sync": "'$(date)'"}}}'

	# find all failed pods and delete them
	%[1]s fd pods --status failed -delete -A
`

	errNoContext = fmt.Errorf(
		"no context is currently set, use %q to select a new one",
		"kubectl config use-context <context>",
	)
)

const defaultDrainChunkSize int64 = 500

// FindOptions provides information required to handle the `find` command.
type FindOptions struct {
	configFlags *genericclioptions.ConfigFlags

	userSpecifiedNamespace string
	excludedNamespace      string
	namespaceSpecified     bool

	rawConfig api.Config
	rest      *rest.Config

	discoveryClient *discovery.DiscoveryClient
	resourceMapper  meta.RESTMapper

	allNamespaces bool
	searchType    string
	delete        bool
	cordon        bool
	uncordon      bool
	drain         bool
	restart       bool
	exec          string
	patch         string
	annotate      string
	regex         string
	podStatus     string
	minAge        string
	maxAge        string
	labelSelector string
	nodeNameRegex string
	skipConfirm   bool
	force         bool
	restarted     bool
	imageRegex    string
	controller    string
	jqFilter      string
	naturalSort   bool
	not           bool

	drainIgnoreDaemonSets      bool
	drainDeleteEmptyDirData    bool
	drainGracePeriodSeconds    int
	drainTimeout               time.Duration
	drainPodSelector           string
	drainDisableEviction       bool
	drainSkipWaitDeleteTimeout int
	drainChunkSize             int64

	excludedRegex         string
	excludedLabelSelector string
	excludedPodStatus     string
	excludedNodeNameRegex string
	excludedImageRegex    string
	excludedController    string
	excludedJQFilter      string
	excludeRestarted      bool

	nodeConditions []string

	showNodeLabels  []string
	showLabels      []string
	showAnnotations []string
	output          string

	args []string

	resourceType handlers.Resource
	handler      handlers.ResourceHandler
	options      handlers.ActionOptions

	genericiooptions.IOStreams
}

// negatableStringValue directs values to either a normal flag or its exclusion
// value based on whether --not has already appeared on the command line.
type negatableStringValue struct {
	value    *string
	excluded *string
	not      *bool
}

func (v *negatableStringValue) String() string {
	if v.value == nil {
		return ""
	}
	return *v.value
}

func (v *negatableStringValue) Set(value string) error {
	if *v.not {
		*v.excluded = value
		return nil
	}
	*v.value = value
	return nil
}

func (*negatableStringValue) Type() string { return "string" }

func newNegatableStringValue(value *string, excluded *string, not *bool) pflag.Value {
	return &negatableStringValue{value: value, excluded: excluded, not: not}
}

type negatableBoolValue struct {
	value    *bool
	excluded *bool
	not      *bool
}

func (v *negatableBoolValue) String() string { return strconv.FormatBool(*v.value) }

func (v *negatableBoolValue) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if *v.not {
		*v.excluded = parsed
		return nil
	}
	*v.value = parsed
	return nil
}

func (*negatableBoolValue) Type() string     { return "bool" }
func (*negatableBoolValue) IsBoolFlag() bool { return true }

type negatablePFlagValue struct {
	value    pflag.Value
	excluded *string
	included *bool
	negated  *bool
}

func (v *negatablePFlagValue) String() string { return v.value.String() }
func (v *negatablePFlagValue) Type() string   { return v.value.Type() }
func (v *negatablePFlagValue) Set(value string) error {
	if *v.negated {
		*v.excluded = value
		return nil
	}
	*v.included = true
	return v.value.Set(value)
}

type outputFormatValue struct {
	output *string
}

func (v *outputFormatValue) String() string { return *v.output }

func (v *outputFormatValue) Set(value string) error {
	if err := printers.ValidateOutputFormat(value); err != nil {
		return err
	}
	*v.output = value
	return nil
}

func (*outputFormatValue) Type() string { return "string" }

// NewFindOptions provides an instance of FindOptions with default values.
func NewFindOptions(streams genericiooptions.IOStreams) *FindOptions {
	return &FindOptions{
		configFlags: genericclioptions.NewConfigFlags(true),

		IOStreams:  streams,
		searchType: "pods",
	}
}

// NewCmdFind provides a cobra command wrapping FindOptions.
func NewCmdFind(streams genericiooptions.IOStreams) *cobra.Command {
	o := NewFindOptions(streams)
	return newCmdFind(o)
}

// newCmdFind builds the command with the supplied options. Keeping construction
// separate lets command-level tests verify Cobra's argument parsing directly.
func newCmdFind(o *FindOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "fd [resource type] [flags]",
		Short:        "Find kubernetes resources and perform actions on them",
		Example:      fmt.Sprintf(findExample, "kubectl"),
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		Annotations: map[string]string{
			cobra.CommandDisplayNameAnnotation: "kubectl fd",
		},
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().
		VarP(newNegatableStringValue(&o.regex, &o.excludedRegex, &o.not), "name", "r", "Regular expression to match resource names against; if not specified, all resources of the specified type will be returned.")
	cmd.Flags().
		Var(newNegatableStringValue(&o.podStatus, &o.excludedPodStatus, &o.not), "status", "Filter pods by their status (phase); e.g. 'Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'.")
	cmd.Flags().
		BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "Search in all namespaces; if not specified, only the current namespace will be searched.")
	cmd.Flags().
		VarP(newNegatableStringValue(&o.labelSelector, &o.excludedLabelSelector, &o.not), "selector", "l", "Label selector to filter resources by labels.")
	cmd.Flags().BoolVar(&o.not, "not", false, "Negate every following filter flag.")
	cmd.Flags().BoolVar(&o.delete, "delete", false, "Delete all matched resources.")
	cmd.Flags().BoolVar(&o.cordon, "cordon", false, "Cordon all matched nodes.")
	cmd.Flags().BoolVar(&o.uncordon, "uncordon", false, "Uncordon all matched nodes.")
	cmd.Flags().BoolVar(&o.drain, "drain", false, "Cordon and drain all matched nodes.")
	cmd.Flags().BoolVar(&o.restart, "restart", false, "Restart all matched deployments, daemonsets, or statefulsets.")
	cmd.Flags().StringVarP(&o.exec, "exec", "e", "", "Execute a command on all found pods.")
	cmd.Flags().StringVarP(&o.patch, "patch", "p", "", "Patch all found resources with the specified JSON patch.")
	cmd.Flags().StringVar(&o.annotate, "annotate", "",
		"Annotate all found resources; format: k=v[,k2=v2] to add/overwrite or k- to remove annotations.")
	cmd.Flags().
		StringVar(&o.minAge, "min-age", "", "Filter resources by minimum age; e.g. '2d' for 2 days, '3h' for 3 hours, etc.")
	cmd.Flags().
		StringVar(&o.maxAge, "max-age", "", "Filter resources by maximum age; e.g. '2d' for 2 days, '3h' for 3 hours, etc.")
	cmd.Flags().
		BoolVarP(&o.skipConfirm, "skip-confirm", "y", false, "Skip confirmation prompt before performing actions on resources.")
	cmd.Flags().
		BoolVar(&o.drainIgnoreDaemonSets, "drain-ignore-daemonsets", false, "Ignore DaemonSet-managed pods while draining nodes.")
	cmd.Flags().
		BoolVar(&o.drainDeleteEmptyDirData, "drain-delete-emptydir-data", false, "Continue draining when pods use emptyDir data.")
	cmd.Flags().
		IntVar(&o.drainGracePeriodSeconds, "drain-grace-period", -1, "Grace period in seconds for pods drained from nodes; negative uses the pod default.")
	cmd.Flags().
		DurationVar(&o.drainTimeout, "drain-timeout", 0, "Maximum time to wait for each node drain; zero means infinite.")
	cmd.Flags().
		StringVar(&o.drainPodSelector, "drain-pod-selector", "", "Label selector to filter pods drained from nodes.")
	cmd.Flags().
		BoolVar(&o.drainDisableEviction, "drain-disable-eviction", false, "Use deletion instead of eviction when draining nodes.")
	cmd.Flags().
		IntVar(&o.drainSkipWaitDeleteTimeout, "drain-skip-wait-for-delete-timeout", 0, "Skip waiting for pods with a deletion timestamp older than this many seconds.")
	cmd.Flags().
		Int64Var(&o.drainChunkSize, "drain-chunk-size", defaultDrainChunkSize, "Number of pods to fetch per request while draining nodes; 0 disables chunking.")
	cmd.Flags().
		BoolVar(&o.force, "force", false, "If true, immediately remove resources from API and bypass graceful deletion. Can only be used with --delete flag.")
	cmd.Flags().
		Var(newNegatableStringValue(&o.nodeNameRegex, &o.excludedNodeNameRegex, &o.not), "node", "Filter pods by node name regex; Uses pod.Spec.NodeName or pod.Status.NominatedNodeName if the former is empty.")
	cmd.Flags().
		Var(newNegatableStringValue(&o.nodeNameRegex, &o.excludedNodeNameRegex, &o.not), "host", "Alias for --node.")
	cmd.Flags().
		Var(&negatableBoolValue{value: &o.restarted, excluded: &o.excludeRestarted, not: &o.not}, "restarted", "Find pods that have been restarted at least once.")
	cmd.Flags().Lookup("restarted").NoOptDefVal = "true"
	cmd.Flags().
		Var(newNegatableStringValue(&o.imageRegex, &o.excludedImageRegex, &o.not), "image", "Regular expression to match container images against.")
	cmd.Flags().
		Var(newNegatableStringValue(&o.controller, &o.excludedController, &o.not), "controller", "Filter pods by their direct controller; format: API-GROUP/RESOURCE (e.g. apps/daemonsets).")
	cmd.Flags().
		VarP(newNegatableStringValue(&o.jqFilter, &o.excludedJQFilter, &o.not), "jq", "j", "jq expression to filter resources; Uses gojq library for evaluation.")
	cmd.Flags().
		StringSliceVarP(&o.showNodeLabels, "node-labels", "N", nil, "Comma-separated list of node labels to show.")
	cmd.Flags().
		StringSliceVarP(&o.showLabels, "labels", "L", nil, "Comma-separated list of labels to show.")
	cmd.Flags().
		StringSliceVarP(&o.showAnnotations, "annotations", "T", nil, "Comma-separated list of annotations to show.")
	cmd.Flags().
		BoolVar(&o.naturalSort, "natural-sort", false, "Sort resource names in natural order.")
	cmd.Flags().StringSliceVar(&o.nodeConditions, "node-condition", nil,
		"Filter nodes by conditions; format: ConditionType=Status (e.g. 'Ready=True', 'SchedulingDisabled=False'). Supports custom conditions from NPD or other agents.")
	cmd.Flags().
		VarP(&outputFormatValue{output: &o.output}, "output", "o", "Output format: json, yaml, kyaml, name, go-template=EXPR (or gotemplate=EXPR), or jsonpath=EXPR.")

	o.configFlags.AddFlags(cmd.Flags())
	namespaceFlag := cmd.Flags().Lookup("namespace")
	namespaceFlag.Value = &negatablePFlagValue{
		value:    namespaceFlag.Value,
		excluded: &o.excludedNamespace,
		included: &o.namespaceSpecified,
		negated:  &o.not,
	}

	return cmd
}

// Complete sets all information required for updating the current context.
func (o *FindOptions) Complete(cmd *cobra.Command, args []string) error {
	o.args = args

	if len(o.args) > 0 {
		o.searchType = o.args[0]
	}

	var err error
	loader := o.configFlags.ToRawKubeConfigLoader()
	o.rest, err = loader.ClientConfig()
	if err != nil {
		return fmt.Errorf("unable to create REST config: %w", err)
	}

	o.rawConfig, err = loader.RawConfig()
	if err != nil {
		return fmt.Errorf("unable to retrieve raw kubeconfig: %w", err)
	}

	currentContext, exists := o.rawConfig.Contexts[o.rawConfig.CurrentContext]
	if !exists {
		return errNoContext
	}

	o.userSpecifiedNamespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return fmt.Errorf("unable to retrieve namespace flag value: %w", err)
	}

	if o.userSpecifiedNamespace != "" && o.allNamespaces {
		return errors.New("cannot specify both --namespace and --all-namespaces flags")
	}
	if o.excludedNamespace != "" && !o.namespaceSpecified {
		o.allNamespaces = true
		o.userSpecifiedNamespace = ""
	}

	// if no namespace argument or flag value was specified, then use the current context's namespace
	if len(o.userSpecifiedNamespace) == 0 {
		if o.allNamespaces {
			o.userSpecifiedNamespace = ""
		} else {
			o.userSpecifiedNamespace = currentContext.Namespace
			if len(o.userSpecifiedNamespace) == 0 {
				o.userSpecifiedNamespace = "default" // default namespace if none is specified
			}
		}
	}

	return nil
}

func cleanResourceName(resource string) string {
	if strings.Contains(resource, ".") {
		// If the resource contains a dot, it is likely a namespaced resource like "pods.v1"
		// We only want the resource name part, so we split by dot and take the first part.
		return strings.Split(resource, ".")[0]
	}

	if strings.Contains(resource, "/") {
		return strings.Split(resource, "/")[0]
	}
	return resource
}

func (o *FindOptions) initializeDiscoveryClientAndRESTMapper() error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(o.rest)
	if err != nil {
		return fmt.Errorf("unable to create discovery client: %w", err)
	}
	discoveryCachedClient := memory.NewMemCacheClient(discoveryClient)
	restMapper := restmapper.NewShortcutExpander(
		restmapper.NewDeferredDiscoveryRESTMapper(discoveryCachedClient),
		discoveryClient,
		nil, // no warning handler
	)
	o.discoveryClient = discoveryClient
	o.resourceMapper = restMapper

	return nil
}

func (o *FindOptions) findResource(resource string) (handlers.Resource, error) {
	empty := handlers.Resource{}
	resource = cleanResourceName(resource)

	gvr := schema.GroupVersionResource{Resource: resource}

	resolved, err := o.resourceMapper.ResourceFor(gvr)
	if err != nil {
		return empty, fmt.Errorf("unable to resolve resource %s: %w", resource, err)
	}

	gvk, err := o.resourceMapper.KindFor(resolved)
	if err != nil {
		return empty, fmt.Errorf("unable to get kind for resource %q: %w", resource, err)
	}

	groupVersion := resolved.GroupVersion().String()

	apiResourceList, err := o.discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		return empty, fmt.Errorf("unable to get server resources for group version %q: %w", groupVersion, err)
	}

	for _, resource := range apiResourceList.APIResources {
		if resource.Name == resolved.Resource {
			return handlers.Resource{
				GroupVersionResource: resolved,
				PluralName:           resource.Name,
				SingularName:         resource.SingularName,
				IsNamespaced:         resource.Namespaced,
				GroupVersionKind:     gvk,
			}, nil
		}
	}
	return empty, fmt.Errorf("resource %q not found in group version %q", resource, groupVersion)
}

// findController resolves an API group and resource name to the GroupKind
// stored by a pod's controller owner reference.
func (o *FindOptions) findController(controller string) (schema.GroupKind, error) {
	parts := strings.Split(controller, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return schema.GroupKind{}, fmt.Errorf(
			"invalid controller %q, expected API-GROUP/RESOURCE (e.g. apps/daemonsets)",
			controller,
		)
	}

	resource := schema.GroupVersionResource{
		Group:    parts[0],
		Resource: strings.ToLower(parts[1]),
	}
	resolved, err := o.resourceMapper.ResourceFor(resource)
	if err != nil {
		return schema.GroupKind{}, fmt.Errorf("unable to resolve controller %q: %w", controller, err)
	}
	kind, err := o.resourceMapper.KindFor(resolved)
	if err != nil {
		return schema.GroupKind{}, fmt.Errorf("unable to resolve controller kind %q: %w", controller, err)
	}

	return kind.GroupKind(), nil
}

// Validate ensures that all required arguments and flag values are provided.
func (o *FindOptions) Validate() error {
	if len(o.rawConfig.CurrentContext) == 0 {
		return errNoContext
	}

	if err := o.initializeDiscoveryClientAndRESTMapper(); err != nil {
		return err
	}

	var err error
	o.resourceType, err = o.findResource(o.searchType)
	if err != nil {
		return fmt.Errorf("unable to find resource type %q: %w", o.searchType, err)
	}

	clientSet, err := kubernetes.NewForConfig(o.rest)
	if err != nil {
		return fmt.Errorf("unable to create kubernetes client: %w", err)
	}

	dynamic, err := dynamic.NewForConfig(o.rest)
	if err != nil {
		return fmt.Errorf("unable to create dynamic client: %w", err)
	}

	o.handler, err = handlers.GetResourceHandler(
		o.resourceType,
		handlers.NewHandlerOptions().
			WithClientSet(clientSet).
			WithNamespaced(o.allNamespaces).
			WithRestarted(o.restarted).
			WithDynamic(dynamic).
			WithImages(o.imageRegex != "").
			WithLabels(o.showLabels).
			WithNodeLabels(o.showNodeLabels).
			WithAnnotations(o.showAnnotations).
			WithOutput(o.output).
			WithExecutorGetter(func(method string, url *url.URL) (remotecommand.Executor, error) {
				return remotecommand.NewSPDYExecutor(
					o.rest,
					method,
					url,
				)
			}),
	)
	if err != nil {
		return fmt.Errorf("unable to create resource handler for type %s: %w", o.resourceType.SingularName, err)
	}

	if o.handler == nil {
		return fmt.Errorf("no handler found for resource type %s", o.resourceType.SingularName)
	}

	action := handlers.ActionList
	if o.delete {
		action = handlers.ActionDelete
	}
	if o.patch != "" {
		if o.delete {
			return errors.New("cannot specify both --delete and --patch flags")
		}
		action = handlers.ActionPatch
	}
	if o.exec != "" {
		if o.delete || o.patch != "" {
			return errors.New("cannot specify both --delete or --patch and --exec flags")
		}
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("exec action is only supported for pods, but got %q", o.resourceType.PluralName)
		}
		action = handlers.ActionExec
	}
	if o.cordon || o.uncordon || o.drain {
		if o.delete || o.patch != "" || o.exec != "" || o.annotate != "" || o.restart {
			return errors.New("cannot combine node actions with other actions")
		}
		if (o.cordon && o.uncordon) || (o.cordon && o.drain) || (o.uncordon && o.drain) {
			return errors.New("only one node action may be specified")
		}
		if o.resourceType.GroupVersionResource != handlers.NodeType {
			return fmt.Errorf("node actions are only supported for nodes, but got %q", o.resourceType.PluralName)
		}
		switch {
		case o.cordon:
			action = handlers.ActionCordon
		case o.uncordon:
			action = handlers.ActionUncordon
		case o.drain:
			action = handlers.ActionDrain
		}
	}
	if o.restart {
		if o.delete || o.patch != "" || o.exec != "" || o.annotate != "" {
			return errors.New("cannot combine --restart with other actions")
		}
		if !handlers.SupportsRolloutRestart(o.resourceType.GroupVersionResource) {
			return fmt.Errorf(
				"restart action is only supported for deployments, daemonsets, and statefulsets, but got %q",
				o.resourceType.PluralName,
			)
		}
		action = handlers.ActionRestart
	}

	var annotateCfg handlers.AnnotateConfig
	if o.annotate != "" {
		if o.delete || o.patch != "" || o.exec != "" || o.restart {
			return errors.New("cannot combine --annotate with --delete, --patch, --exec, or --restart flags")
		}
		var err2 error
		annotateCfg, err2 = handlers.ParseAnnotateFlag(o.annotate)
		if err2 != nil {
			return fmt.Errorf("invalid --annotate flag value: %w", err2)
		}
		action = handlers.ActionAnnotate
	}

	if o.force && action != handlers.ActionDelete && action != handlers.ActionDrain {
		return errors.New("--force flag can only be used with --delete or --drain")
	}

	if action == handlers.ActionExec && !o.handler.IsExecutable() {
		return fmt.Errorf("resource type %q does not support execution",
			o.resourceType.GroupVersionResource.String())
	}

	var reg *regexp.Regexp
	if o.regex != "" {
		reg, err = regexp.Compile(o.regex)
		if err != nil {
			return fmt.Errorf("invalid regex %q: %w", o.regex, err)
		}
	}
	var excludedRegex *regexp.Regexp
	if o.excludedRegex != "" {
		excludedRegex, err = regexp.Compile(o.excludedRegex)
		if err != nil {
			return fmt.Errorf("invalid excluded regex %q: %w", o.excludedRegex, err)
		}
	}

	var minAge, maxAge time.Duration

	if o.minAge != "" {
		if minAge, err = time.ParseDuration(o.minAge); err != nil {
			return fmt.Errorf("invalid minimum age %q: %w", o.minAge, err)
		}
	}
	if o.maxAge != "" {
		if maxAge, err = time.ParseDuration(o.maxAge); err != nil {
			return fmt.Errorf("invalid maximum age %q: %w", o.maxAge, err)
		}
	}

	if o.podStatus != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("status filtering is only supported for pods, but got %q",
				o.resourceType.GroupVersionResource.String())
		}
		if !handlers.IsValidPodStatus(o.podStatus) {
			return fmt.Errorf("invalid pod status %q, must be one of: %v", o.podStatus, handlers.ValidPodStatuses)
		}
	}
	var excludedPodStatus v1.PodPhase
	if o.excludedPodStatus != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf(
				"status filtering is only supported for pods, but got %q",
				o.resourceType.GroupVersionResource.String(),
			)
		}
		if !handlers.IsValidPodStatus(o.excludedPodStatus) {
			return fmt.Errorf(
				"invalid pod status %q, must be one of: %v",
				o.excludedPodStatus,
				handlers.ValidPodStatuses,
			)
		}
		excludedPodStatus = handlers.ToPodPhase(o.excludedPodStatus)
	}

	if o.showNodeLabels != nil && o.resourceType.GroupVersionResource != handlers.PodType {
		return fmt.Errorf("showing node labels is only supported for pods, but got %q",
			o.resourceType.GroupVersionResource.String())
	}

	var nodeNameRegex *regexp.Regexp
	if o.nodeNameRegex != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("node filtering is only supported for pods, but got %q",
				o.resourceType.GroupVersionResource.String())
		}
		if nodeNameRegex, err = regexp.Compile(o.nodeNameRegex); err != nil {
			return fmt.Errorf("invalid node name regex filter %q: %w", o.nodeNameRegex, err)
		}
	}
	var excludedNodeNameRegex *regexp.Regexp
	if o.excludedNodeNameRegex != "" {
		excludedNodeNameRegex, err = regexp.Compile(o.excludedNodeNameRegex)
		if err != nil {
			return fmt.Errorf("invalid excluded node name regex %q: %w", o.excludedNodeNameRegex, err)
		}
	}
	if excludedNodeNameRegex != nil && o.resourceType.GroupVersionResource != handlers.PodType {
		return fmt.Errorf(
			"node filtering is only supported for pods, but got %q",
			o.resourceType.GroupVersionResource.String(),
		)
	}

	var imagesRegex *regexp.Regexp
	if o.imageRegex != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("image filtering is only supported for pods, but got %q",
				o.resourceType.GroupVersionResource.String())
		}
		if imagesRegex, err = regexp.Compile(o.imageRegex); err != nil {
			return fmt.Errorf("invalid image regex filter %q: %w", o.imageRegex, err)
		}
	}
	var excludedImageRegex *regexp.Regexp
	if o.excludedImageRegex != "" {
		excludedImageRegex, err = regexp.Compile(o.excludedImageRegex)
		if err != nil {
			return fmt.Errorf("invalid excluded image regex %q: %w", o.excludedImageRegex, err)
		}
	}
	if excludedImageRegex != nil && o.resourceType.GroupVersionResource != handlers.PodType {
		return fmt.Errorf(
			"image filtering is only supported for pods, but got %q",
			o.resourceType.GroupVersionResource.String(),
		)
	}

	var controller, excludedController *schema.GroupKind
	if o.controller != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("controller filtering is only supported for pods, but got %q",
				o.resourceType.GroupVersionResource.String())
		}
		resolved, resolveErr := o.findController(o.controller)
		if resolveErr != nil {
			return resolveErr
		}
		controller = &resolved
	}
	if o.excludedController != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("controller filtering is only supported for pods, but got %q",
				o.resourceType.GroupVersionResource.String())
		}
		resolved, resolveErr := o.findController(o.excludedController)
		if resolveErr != nil {
			return resolveErr
		}
		excludedController = &resolved
	}
	var jqQuery *gojq.Query
	if o.jqFilter != "" {
		jqQuery, err = pkg.PrepareQuery(o.jqFilter)
		if err != nil {
			return fmt.Errorf("invalid jq filter %q: %w", o.jqFilter, err)
		}
		if jqQuery == nil {
			return fmt.Errorf("invalid jq filter %q", o.jqFilter)
		}
	}
	var excludedJQQuery *gojq.Query
	if o.excludedJQFilter != "" {
		excludedJQQuery, err = pkg.PrepareQuery(o.excludedJQFilter)
		if err != nil || excludedJQQuery == nil {
			if err != nil {
				return fmt.Errorf("invalid jq filter %q: %w", o.excludedJQFilter, err)
			}
			return fmt.Errorf("invalid jq filter %q", o.excludedJQFilter)
		}
	}

	var excludedLabelSelector labels.Selector
	if o.excludedLabelSelector != "" {
		parsed, parseErr := labels.Parse(o.excludedLabelSelector)
		if parseErr != nil {
			return fmt.Errorf("invalid excluded label selector %q: %w", o.excludedLabelSelector, parseErr)
		}
		excludedLabelSelector = parsed
	}

	var nodeConditions []handlers.NodeCondition
	if len(o.nodeConditions) > 0 {
		if o.resourceType.GroupVersionResource != handlers.NodeType {
			return fmt.Errorf(
				"node condition filtering is only supported for nodes, but got %q",
				o.resourceType.GroupVersionResource.String(),
			)
		}
		for _, nc := range o.nodeConditions {
			parts := strings.SplitN(nc, "=", 2) //nolint:mnd // split into key=value pair
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf(
					"invalid node condition format %q, expected ConditionType=Status (e.g. Ready=True)",
					nc,
				)
			}
			nodeConditions = append(nodeConditions, handlers.NodeCondition{
				Type:   parts[0],
				Status: parts[1],
			})
		}
	}

	o.options = handlers.ActionOptions{
		Namespace:                  o.userSpecifiedNamespace,
		ExcludedNamespace:          o.excludedNamespace,
		Action:                     action,
		NameRegex:                  reg,
		ExcludedNameRegex:          excludedRegex,
		MaxAge:                     maxAge,
		MinAge:                     minAge,
		LabelSelector:              o.labelSelector, // todo: add validation for label selector
		ExcludedLabelSelector:      excludedLabelSelector,
		Streams:                    &o.IOStreams,
		JQQuery:                    jqQuery,
		ExcludedJQQuery:            excludedJQQuery,
		NodeNameRegex:              nodeNameRegex,
		ExcludedNodeNameRegex:      excludedNodeNameRegex,
		SkipConfirm:                o.skipConfirm,
		Force:                      o.force,
		PodStatus:                  handlers.ToPodPhase(o.podStatus),
		ExcludedPodStatus:          excludedPodStatus,
		Exec:                       o.exec,
		Patch:                      o.patch,
		Annotate:                   annotateCfg,
		ResourceType:               o.resourceType,
		Restarted:                  o.restarted,
		ExcludeRestarted:           o.excludeRestarted,
		ImageRegex:                 imagesRegex,
		ExcludedImageRegex:         excludedImageRegex,
		ShowNodeLabels:             o.showNodeLabels,
		ShowLabels:                 o.showLabels,
		ShowAnnotations:            o.showAnnotations,
		NaturalSort:                o.naturalSort,
		NodeConditions:             nodeConditions,
		DrainIgnoreDaemonSets:      o.drainIgnoreDaemonSets,
		DrainDeleteEmptyDirData:    o.drainDeleteEmptyDirData,
		DrainGracePeriodSeconds:    o.drainGracePeriodSeconds,
		DrainTimeout:               o.drainTimeout,
		DrainPodSelector:           o.drainPodSelector,
		DrainDisableEviction:       o.drainDisableEviction,
		DrainSkipWaitDeleteTimeout: o.drainSkipWaitDeleteTimeout,
		DrainChunkSize:             o.drainChunkSize,
		Controller:                 controller,
		ExcludedController:         excludedController,
	}

	return nil
}

type Title string

func (t Title) Format() string {
	return strings.ToUpper(string(t))
}

// Run finds all resources of a specified type matching the provided criteria
// and optionally performs an action on them.
func (o *FindOptions) Run() error {
	ctx := context.Background()

	return o.handler.HandleAction(ctx, o.options)
}
