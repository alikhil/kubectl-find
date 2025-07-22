package types

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/alikhil/kubectl-find/pkg/prompts"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8s_types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	ValidPodStatuses = []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}
)

func IsValidPodStatus(status string) bool {
	status = strings.ToLower(status)
	for _, validStatus := range ValidPodStatuses {
		if status == strings.ToLower(validStatus) {
			return true
		}
	}
	return false
}

func ToPodPhase(status string) v1.PodPhase {
	switch strings.ToLower(status) {
	case "pending":
		return v1.PodPending
	case "running":
		return v1.PodRunning
	case "succeeded":
		return v1.PodSucceeded
	case "failed":
		return v1.PodFailed
	case "unknown":
		return v1.PodUnknown
	default:
		return ""
	}
}

type ExecutorGetter func(method string, url *url.URL) (remotecommand.Executor, error)

type PodHandler struct {
	clientSet      kubernetes.Interface
	executorGetter ExecutorGetter
	printer        printers.BatchPrinter
}

// HandleAction implements ResourceHandler.
func (p *PodHandler) HandleAction(ctx context.Context, options ActionOptions) error {

	matcher, err := p.getMatcher(options)
	if err != nil {
		return fmt.Errorf("failed to get matcher: %w", err)
	}

	pods, err := p.clientSet.CoreV1().Pods(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: options.LabelSelector})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	matchedPods := make([]*v1.Pod, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if matcher(&pod) {
			matchedPods = append(matchedPods, &pod)
		}
	}

	if len(matchedPods) == 0 {
		return nil
	}

	if options.Action == ActionList {
		unstructuredPods := make([]unstructured.Unstructured, len(matchedPods))
		for i, pod := range matchedPods {
			unstr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
			if err != nil {
				return fmt.Errorf("failed to convert pod %s to unstructured: %w", pod.Name, err)
			}
			unstructuredPods[i] = unstructured.Unstructured{Object: unstr}
		}

		return p.printer.PrintObjects(unstructuredPods, options.Streams.Out)
	} else if options.Action == ActionDelete {

		if !options.SkipConfirm {
			options.Streams.ErrOut.Write([]byte("The following pods will be deleted:\n"))
			for _, pod := range matchedPods {
				options.Streams.ErrOut.Write([]byte(fmt.Sprintf("- %s in namespace %s\n", pod.Name, pod.Namespace)))
			}
			if !prompts.AskForConfirmation(options.Streams) {
				options.Streams.ErrOut.Write([]byte("Deletion cancelled.\n"))
				return nil
			}
		}
		for _, pod := range matchedPods {
			deletionPropagation := metav1.DeletePropagationBackground
			err = p.clientSet.CoreV1().Pods(pod.ObjectMeta.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{PropagationPolicy: &deletionPropagation})
			if err != nil {
				return fmt.Errorf("failed to delete pod %s: %w", pod.Name, err)
			}
			options.Streams.Out.Write([]byte(fmt.Sprintf("Deleted pod %s in namespace %s\n", pod.Name, pod.Namespace)))
		}

		return nil
	} else if options.Action == ActionPatch {
		if options.Patch == "" {
			return fmt.Errorf("patch content is required for patch action")
		}
		if !options.SkipConfirm {
			options.Streams.ErrOut.Write([]byte("The following pods will be patched:\n"))
			for _, pod := range matchedPods {
				options.Streams.ErrOut.Write([]byte(fmt.Sprintf("- %s in namespace %s\n", pod.Name, pod.Namespace)))
			}
			if !prompts.AskForConfirmation(options.Streams) {
				options.Streams.ErrOut.Write([]byte("Patch cancelled.\n"))
				return nil
			}
		}
		for _, pod := range matchedPods {
			_, err = p.clientSet.CoreV1().Pods(pod.ObjectMeta.Namespace).Patch(ctx, pod.Name, k8s_types.StrategicMergePatchType, []byte(options.Patch), metav1.PatchOptions{})
			if err != nil {
				return fmt.Errorf("failed to patch pod %s: %w", pod.Name, err)
			}
			options.Streams.Out.Write([]byte(fmt.Sprintf("Patched pod %s in namespace %s\n", pod.Name, pod.Namespace)))
		}
	} else if options.Action == ActionExec {
		if options.Exec == "" {
			return fmt.Errorf("exec command is required for exec action")
		}
		if !options.SkipConfirm {
			options.Streams.ErrOut.Write([]byte("The following pods will have the command executed:\n"))
			for _, pod := range matchedPods {
				options.Streams.ErrOut.Write([]byte(fmt.Sprintf("- %s in namespace %s\n", pod.Name, pod.Namespace)))
			}
			if !prompts.AskForConfirmation(options.Streams) {
				options.Streams.ErrOut.Write([]byte("Execution cancelled.\n"))
				return nil
			}
		}
		for _, pod := range matchedPods {
			rest := p.clientSet.CoreV1().RESTClient().
				Post().
				Resource("pods").
				Name(pod.Name).
				Namespace(pod.Namespace).
				SubResource("exec").
				VersionedParams(&v1.PodExecOptions{
					Command: strings.Split(options.Exec, " "),
					Stdin:   false,
					Stdout:  true,
					Stderr:  true,
					TTY:     false,
				}, scheme.ParameterCodec)

			exec, err := p.executorGetter(
				"POST",
				rest.URL(),
			)

			if err != nil {
				return fmt.Errorf("failed to create executor for pod %s: %w", pod.Name, err)
			}

			err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
				Stdin:  nil,
				Stdout: options.Streams.Out,
				Stderr: options.Streams.ErrOut,
				Tty:    false,
			})
			if err != nil {
				return fmt.Errorf("failed to execute command on pod %s: %w", pod.Name, err)
			}

		}
	} else {
		panic("unimplemented action")
	}
	return nil
}

func (p *PodHandler) getMatcher(opts ActionOptions) (func(pod *v1.Pod) bool, error) {
	var regex = opts.NameRegex

	return func(pod *v1.Pod) bool {
		if regex != nil && !regex.MatchString(pod.Name) {
			return false
		}
		if opts.MinAge != 0 {
			if time.Since(pod.CreationTimestamp.Time) < opts.MinAge {
				return false
			}
		}
		if opts.MaxAge != 0 {
			if time.Since(pod.CreationTimestamp.Time) > opts.MaxAge {
				return false
			}
		}
		if opts.PodStatus != "" {
			if pod.Status.Phase != opts.PodStatus {
				return false
			}
		}
		return true
	}, nil
}

// GetPropertyHeadersToDisplay implements ResourceHandler.
func (p *PodHandler) GetPropertyHeadersToDisplay() (headers []string) {
	return []string{"Name", "Status", "Age"}
}

// GetPropertyValuesToDisplay implements ResourceHandler.
func (p *PodHandler) GetPropertyValuesToDisplay() (values []string) {
	panic("unimplemented")
}

// IsDeletable implements ResourceHandler.
func (p *PodHandler) IsDeletable() bool {
	return true
}

// IsExecutable implements ResourceHandler.
func (p *PodHandler) IsExecutable() bool {
	return true
}

// IsGlobal implements ResourceHandler.
func (p *PodHandler) IsGlobal() bool {
	return false
}

// IsPatchable implements ResourceHandler.
func (p *PodHandler) IsPatchable() bool {
	return true
}
