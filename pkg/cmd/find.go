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
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"

	"github.com/alikhil/kubectl-find/pkg"
	"github.com/alikhil/kubectl-find/pkg/handlers"
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
	%[1]s find --name mypod-*

	# find secrets created more than 2 days ago in specified namespace
	%[1]s find secrets --min-age 2d -n superapp

	# find all externalecrets and patch them to force sync
	%[1]s find externalsecret -A --patch '{"metadata": {"annotations": {"force-sync": "'$(date)'"}}}'

	# find all failed pods and delete them
	%[1]s find pods --status failed -delete -A
`

	errNoContext = fmt.Errorf(
		"no context is currently set, use %q to select a new one",
		"kubectl config use-context <context>",
	)
)

// FindOptions provides information required to handle the `find` command.
type FindOptions struct {
	configFlags *genericclioptions.ConfigFlags

	userSpecifiedNamespace string

	rawConfig api.Config
	rest      *rest.Config

	allNamespaces bool
	searchType    string
	delete        bool
	exec          string
	patch         string
	regex         string
	podStatus     string
	minAge        string
	maxAge        string
	labelSelector string
	nodeNameRegex string
	skipConfirm   bool
	restarted     bool
	imageRegex    string
	jqFilter      string

	showNodeLabels []string
	showLabels     []string

	args []string

	resourceType handlers.Resource
	handler      handlers.ResourceHandler
	options      handlers.ActionOptions

	genericiooptions.IOStreams
}

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

	cmd := &cobra.Command{
		Use:          "find [resource type] [flags]",
		Short:        "Find kubernetes resources and perform actions on them",
		Example:      fmt.Sprintf(findExample, "kubectl"),
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		Annotations: map[string]string{
			cobra.CommandDisplayNameAnnotation: "kubectl find",
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
		StringVarP(&o.regex, "name", "r", "", "Regular expression to match resource names against; if not specified, all resources of the specified type will be returned.")
	cmd.Flags().
		StringVar(&o.podStatus, "status", "", "Filter pods by their status (phase); e.g. 'Running', 'Pending', 'Succeeded', 'Failed', 'Unknown'.")
	cmd.Flags().
		BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "Search in all namespaces; if not specified, only the current namespace will be searched.")
	cmd.Flags().StringVarP(&o.labelSelector, "selector", "l", "", "Label selector to filter resources by labels.")
	cmd.Flags().BoolVar(&o.delete, "delete", false, "Delete all matched resources.")
	cmd.Flags().StringVarP(&o.exec, "exec", "e", "", "Execute a command on all found pods.")
	cmd.Flags().StringVarP(&o.patch, "patch", "p", "", "Patch all found resources with the specified JSON patch.")
	cmd.Flags().
		StringVar(&o.minAge, "min-age", "", "Filter resources by minimum age; e.g. '2d' for 2 days, '3h' for 3 hours, etc.")
	cmd.Flags().
		StringVar(&o.maxAge, "max-age", "", "Filter resources by maximum age; e.g. '2d' for 2 days, '3h' for 3 hours, etc.")
	cmd.Flags().
		BoolVarP(&o.skipConfirm, "force", "f", false, "Skip confirmation prompt before performing actions on resources.")
	cmd.Flags().
		StringVar(&o.nodeNameRegex, "node", "", "Filter pods by node name regex; Uses pod.Spec.NodeName or pod.Status.NominatedNodeName if the former is empty.")
	cmd.Flags().
		BoolVar(&o.restarted, "restarted", false, "Find pods that have been restarted at least once.")
	cmd.Flags().
		StringVar(&o.imageRegex, "image", "", "Regular expression to match container images against.")
	cmd.Flags().
		StringVarP(&o.jqFilter, "jq", "j", "", "jq expression to filter resources; Uses gojq library for evaluation.")
	cmd.Flags().
		StringSliceVarP(&o.showNodeLabels, "node-labels", "N", nil, "Comma-separated list of node labels to show.")
	cmd.Flags().
		StringSliceVarP(&o.showLabels, "labels", "L", nil, "Comma-separated list of labels to show.")

	o.configFlags.AddFlags(cmd.Flags())

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

func (o *FindOptions) findResource(resource string) (handlers.Resource, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(o.rest)
	empty := handlers.Resource{}
	if err != nil {
		return empty, fmt.Errorf("unable to create discovery client: %w", err)
	}
	discoveryCachedClient := memory.NewMemCacheClient(discoveryClient)

	restMapper := restmapper.NewShortcutExpander(
		restmapper.NewDeferredDiscoveryRESTMapper(discoveryCachedClient),
		discoveryClient,
		nil, // no warning handler
	)

	resource = cleanResourceName(resource)

	gvr := schema.GroupVersionResource{Resource: resource}

	resolved, err := restMapper.ResourceFor(gvr)
	if err != nil {
		return empty, fmt.Errorf("unable to resolve resource %s: %w", resource, err)
	}

	groupVersion := resolved.GroupVersion().String()

	apiResourceList, err := discoveryClient.ServerResourcesForGroupVersion(groupVersion)
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
			}, nil
		}
	}
	return empty, fmt.Errorf("resource %q not found in group version %q", resource, groupVersion)
}

// Validate ensures that all required arguments and flag values are provided.
func (o *FindOptions) Validate() error {
	if len(o.rawConfig.CurrentContext) == 0 {
		return errNoContext
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

	if action == handlers.ActionExec && !o.handler.IsExecutable() {
		return fmt.Errorf("resource type %q does not support execution", o.resourceType)
	}

	var reg *regexp.Regexp
	if o.regex != "" {
		reg, err = regexp.Compile(o.regex)
		if err != nil {
			return fmt.Errorf("invalid regex %q: %w", o.regex, err)
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
			return fmt.Errorf("status filtering is only supported for pods, but got %q", o.resourceType)
		}
		if !handlers.IsValidPodStatus(o.podStatus) {
			return fmt.Errorf("invalid pod status %q, must be one of: %v", o.podStatus, handlers.ValidPodStatuses)
		}
	}

	if o.showNodeLabels != nil && o.resourceType.GroupVersionResource != handlers.PodType {
		return fmt.Errorf("showing node labels is only supported for pods, but got %q", o.resourceType)
	}

	var nodeNameRegex *regexp.Regexp
	if o.nodeNameRegex != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("node filtering is only supported for pods, but got %q", o.resourceType)
		}
		if nodeNameRegex, err = regexp.Compile(o.nodeNameRegex); err != nil {
			return fmt.Errorf("invalid node name regex filter %q: %w", o.nodeNameRegex, err)
		}
	}

	var imagesRegex *regexp.Regexp
	if o.imageRegex != "" {
		if o.resourceType.GroupVersionResource != handlers.PodType {
			return fmt.Errorf("image filtering is only supported for pods, but got %q", o.resourceType)
		}
		if imagesRegex, err = regexp.Compile(o.imageRegex); err != nil {
			return fmt.Errorf("invalid image regex filter %q: %w", o.imageRegex, err)
		}
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

	o.options = handlers.ActionOptions{
		Namespace:      o.userSpecifiedNamespace,
		Action:         action,
		NameRegex:      reg,
		MaxAge:         maxAge,
		MinAge:         minAge,
		LabelSelector:  o.labelSelector, // todo: add validation for label selector
		Streams:        &o.IOStreams,
		JQQuery:        jqQuery,
		NodeNameRegex:  nodeNameRegex,
		SkipConfirm:    o.skipConfirm,
		PodStatus:      handlers.ToPodPhase(o.podStatus),
		Exec:           o.exec,
		Patch:          o.patch,
		ResourceType:   o.resourceType,
		Restarted:      o.restarted,
		ImageRegex:     imagesRegex,
		ShowNodeLabels: o.showNodeLabels,
		ShowLabels:     o.showLabels,
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
