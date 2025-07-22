package types

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/alikhil/kubectl-find/pkg/prompts"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8s_types "k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/dynamic"
)

type UniversalHandler struct {
	opts UniversalHandlerOptions
}

type UniversalHandlerOptions struct {
	Client   dynamic.Interface
	Printer  printers.BatchPrinter
	Resource Resource
}

func NewUniversalHandler(opts UniversalHandlerOptions) *UniversalHandler {
	return &UniversalHandler{opts: opts}
}

func (h *UniversalHandler) IsExecutable() bool {
	return false
}

func (h *UniversalHandler) printResource(resource unstructured.Unstructured, opts ActionOptions, outStream io.Writer) error {
	showNamespace := h.opts.Resource.IsNamespaced && opts.Namespace == ""
	var msg string
	if showNamespace {
		msg = fmt.Sprintf("- %s in namespace %s", resource.GetName(), resource.GetNamespace())
	} else {
		msg = fmt.Sprintf("- %s", resource.GetName())
	}
	_, err := outStream.Write([]byte(msg + "\n"))
	if err != nil {
		return fmt.Errorf("failed to write resource message: %v", err)
	}
	return nil
}

func (h *UniversalHandler) HandleAction(ctx context.Context, options ActionOptions) error {

	// todo: move to another place
	if options.PatchStrategy == "" {
		options.PatchStrategy = k8s_types.StrategicMergePatchType
	}
	var resources dynamic.ResourceInterface
	if h.opts.Resource.IsNamespaced {
		resources = h.opts.Client.Resource(h.opts.Resource.GroupVersionResource).Namespace(options.Namespace)
	} else {
		resources = h.opts.Client.Resource(h.opts.Resource.GroupVersionResource)
	}

	list, err := resources.List(ctx, v1.ListOptions{
		LabelSelector: options.LabelSelector,
	})

	if err != nil {
		return fmt.Errorf("failed to list %s: %v", h.opts.Resource.PluralName, err)
	}

	matchedItems := make([]unstructured.Unstructured, 0, len(list.Items))
	for _, item := range list.Items {
		if h.resourceMatches(item, &options) {
			matchedItems = append(matchedItems, item)
		}
	}
	if len(matchedItems) == 0 {
		return nil
	}

	if options.Action == ActionList {
		return h.opts.Printer.PrintObjects(matchedItems, options.Streams.Out)
	}

	if options.Action == ActionDelete {
		if !options.SkipConfirm {
			options.Streams.ErrOut.Write([]byte(fmt.Sprintf("The following %s will be deleted:\n", h.opts.Resource.PluralName)))
			for _, res := range matchedItems {
				h.printResource(res, options, options.Streams.ErrOut)
			}
			if !prompts.AskForConfirmation(options.Streams) {
				options.Streams.ErrOut.Write([]byte("Deletion cancelled.\n"))
				return nil
			}
		}
		for _, item := range matchedItems {
			deletionPropagation := v1.DeletePropagationBackground

			if err := resources.Delete(ctx, item.GetName(), v1.DeleteOptions{PropagationPolicy: &deletionPropagation}); err != nil {
				return fmt.Errorf("failed to delete %s %s: %v", h.opts.Resource.SingularName, item.GetName(), err)
			}
			options.Streams.Out.Write([]byte(fmt.Sprintf("Deleted %s %s\n", h.opts.Resource.SingularName, item.GetName())))
		}
		return nil
	}
	if options.Action == ActionPatch {
		if options.Patch == "" {
			return fmt.Errorf("patch content is required for patch action")
		}
		if !options.SkipConfirm {
			options.Streams.ErrOut.Write([]byte(fmt.Sprintf("The following %s will be patched:\n", h.opts.Resource.PluralName)))
			for _, res := range matchedItems {
				h.printResource(res, options, options.Streams.ErrOut)
			}
			if !prompts.AskForConfirmation(options.Streams) {
				options.Streams.ErrOut.Write([]byte("Patch cancelled.\n"))
				return nil
			}
		}
		for _, item := range matchedItems {
			patchBytes := []byte(options.Patch)
			_, err := resources.Patch(ctx, item.GetName(), options.PatchStrategy, patchBytes, v1.PatchOptions{})
			if err != nil {
				return fmt.Errorf("failed to patch %s %s: %v", h.opts.Resource.SingularName, item.GetName(), err)
			}
			options.Streams.Out.Write([]byte(fmt.Sprintf("Patched %s %s\n", h.opts.Resource.SingularName, item.GetName())))
		}
		return nil
	}

	return fmt.Errorf("unsupported action: %s", options.Action)
}

func (h *UniversalHandler) resourceMatches(resource unstructured.Unstructured, options *ActionOptions) bool {
	if options.NameRegex != nil && !options.NameRegex.MatchString(resource.GetName()) {
		return false
	}

	if options.MinAge > 0 || options.MaxAge > 0 {
		creationTime := resource.GetCreationTimestamp()
		if options.MinAge > 0 && time.Since(creationTime.Time) < options.MinAge {
			return false
		}
		if options.MaxAge > 0 && time.Since(creationTime.Time) > options.MaxAge {
			return false
		}
	}

	return true
}
