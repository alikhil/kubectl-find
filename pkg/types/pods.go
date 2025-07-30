package types

import (
	"context"
	"errors"
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

//nolint:gochecknoglobals
var ValidPodStatuses = []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}

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
	matcher := p.getMatcher(options)

	pods, err := p.clientSet.CoreV1().
		Pods(options.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: options.LabelSelector})
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

	switch options.Action {
	case ActionList:
		unstructuredPods := make([]unstructured.Unstructured, len(matchedPods))
		for i, pod := range matchedPods {
			var unstr map[string]interface{}
			unstr, err = runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
			if err != nil {
				return fmt.Errorf("failed to convert pod %s to unstructured: %w", pod.Name, err)
			}
			unstructuredPods[i] = unstructured.Unstructured{Object: unstr}
		}

		return p.printer.PrintObjects(unstructuredPods, options.Streams.Out)
	case ActionDelete:
		if !options.SkipConfirm {
			_, err = options.Streams.ErrOut.Write([]byte("The following pods will be deleted:\n"))
			if err != nil {
				return fmt.Errorf("failed to write to error output: %w", err)
			}
			for _, pod := range matchedPods {
				_, err = fmt.Fprintf(options.Streams.ErrOut, "- %s in namespace %s\n", pod.Name, pod.Namespace)
				if err != nil {
					return fmt.Errorf("failed to write to error output: %w", err)
				}
			}
			if !prompts.AskForConfirmation(options.Streams) {
				_, err = options.Streams.ErrOut.Write([]byte("Deletion cancelled.\n"))
				if err != nil {
					return fmt.Errorf("failed to write to error output: %w", err)
				}
				return nil
			}
		}
		for _, pod := range matchedPods {
			deletionPropagation := metav1.DeletePropagationBackground
			err = p.clientSet.CoreV1().
				Pods(pod.ObjectMeta.Namespace).
				Delete(ctx, pod.Name, metav1.DeleteOptions{PropagationPolicy: &deletionPropagation})
			if err != nil {
				return fmt.Errorf("failed to delete pod %s: %w", pod.Name, err)
			}
			_, err = fmt.Fprintf(options.Streams.Out, "Deleted pod %s in namespace %s\n", pod.Name, pod.Namespace)
			if err != nil {
				return fmt.Errorf("failed to write to output: %w", err)
			}
		}

		return nil
	case ActionPatch:
		if options.Patch == "" {
			return errors.New("patch content is required for patch action")
		}
		if !options.SkipConfirm {
			_, err = options.Streams.ErrOut.Write([]byte("The following pods will be patched:\n"))
			if err != nil {
				return fmt.Errorf("failed to write to error output: %w", err)
			}
			for _, pod := range matchedPods {
				_, err = fmt.Fprintf(options.Streams.ErrOut, "- %s in namespace %s\n", pod.Name, pod.Namespace)
				if err != nil {
					return fmt.Errorf("failed to write to error output: %w", err)
				}
			}
			if !prompts.AskForConfirmation(options.Streams) {
				_, err = options.Streams.ErrOut.Write([]byte("Patch cancelled.\n"))
				if err != nil {
					return fmt.Errorf("failed to write to error output: %w", err)
				}
				return nil
			}
		}
		for _, pod := range matchedPods {
			_, err = p.clientSet.CoreV1().
				Pods(pod.ObjectMeta.Namespace).
				Patch(ctx, pod.Name, k8s_types.StrategicMergePatchType, []byte(options.Patch), metav1.PatchOptions{})
			if err != nil {
				return fmt.Errorf("failed to patch pod %s: %w", pod.Name, err)
			}
			_, err = fmt.Fprintf(options.Streams.Out, "Patched pod %s in namespace %s\n", pod.Name, pod.Namespace)
			if err != nil {
				return fmt.Errorf("failed to write to output: %w", err)
			}
		}
	case ActionExec:
		if options.Exec == "" {
			return errors.New("exec command is required for exec action")
		}
		if !options.SkipConfirm {
			_, err = options.Streams.ErrOut.Write([]byte("The following pods will have the command executed:\n"))
			if err != nil {
				return fmt.Errorf("failed to write to error output: %w", err)
			}
			for _, pod := range matchedPods {
				_, err = fmt.Fprintf(options.Streams.ErrOut, "- %s in namespace %s\n", pod.Name, pod.Namespace)
				if err != nil {
					return fmt.Errorf("failed to write to error output: %w", err)
				}
			}
			if !prompts.AskForConfirmation(options.Streams) {
				_, err = options.Streams.ErrOut.Write([]byte("Execution cancelled.\n"))
				if err != nil {
					return fmt.Errorf("failed to write to error output: %w", err)
				}
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

			var exec remotecommand.Executor
			exec, err = p.executorGetter(
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
	default:
		panic("unimplemented action")
	}
	return nil
}

func (p *PodHandler) getMatcher(opts ActionOptions) func(pod *v1.Pod) bool {
	regex := opts.NameRegex

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
		if opts.NodeNameRegex != nil {
			nodeName := pod.Spec.NodeName
			if nodeName == "" {
				nodeName = pod.Status.NominatedNodeName
			}
			if nodeName == "" {
				return false
			}
			if !opts.NodeNameRegex.MatchString(nodeName) {
				return false
			}
		}
		return true
	}
}

// IsExecutable implements ResourceHandler.
func (p *PodHandler) IsExecutable() bool {
	return true
}
