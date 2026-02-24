package handlers

import (
	"bytes"
	"io"
	"regexp"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/alikhil/kubectl-find/pkg"
	"github.com/alikhil/kubectl-find/pkg/mocks"
	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPodsHandler(t *testing.T) {
	type shared struct {
		resources []runtime.Object
		in        *bytes.Buffer
		out       *bytes.Buffer
		errOut    *bytes.Buffer
		streams   genericclioptions.IOStreams
	}

	type fields struct {
		printer        printers.BatchPrinter
		clientSet      kubernetes.Interface
		executorGetter ExecutorGetter
	}
	type args struct {
		options ActionOptions
	}
	type want struct {
		check func(t *testing.T, f *fields, s *shared)
		err   error
	}

	tests := []struct {
		name    string
		prepare func(*testing.T, *fields, *shared) error
		args    args
		want    want
		shared  shared
	}{
		{
			name: "List Pods",
			prepare: func(t *testing.T, f *fields, _ *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.Cond(func(obj []unstructured.Unstructured) bool {
					if len(obj) != 1 {
						return false
					}

					assert.Equal(t, "test-pod", getNestedColumn(t, obj[0], "metadata", "name"))

					return true
				}), gomock.Any()).Return(nil).Times(1)

				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
			},
		},
		{
			name: "List in empty cluster",
			args: args{
				options: ActionOptions{Namespace: "default", Action: ActionList},
			},
			shared: shared{
				resources: []runtime.Object{},
			},
			prepare: func(t *testing.T, f *fields, _ *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.Any(), gomock.Any()).Return(nil).Times(0)

				f.printer = m
				return nil
			},
		},
		{
			name: "List in empty namespace",
			prepare: func(t *testing.T, f *fields, _ *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.Any(), gomock.Any()).Return(nil).Times(0)

				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "nonexistent",
					Action:    ActionList,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				},
			},
		},
		{
			name: "List in all namespaces",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources...)), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "",
					Action:    ActionList,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-2",
							Namespace: "kube-system",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
				},
			},
		},
		{
			name: "List pods matching regex",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().
					PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0:2]...)), gomock.Any()).
					Return(nil).
					Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					NameRegex: regexp.MustCompile(`test\-.*`),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-2",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
		},
		{
			name: "List pods with double regex",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources...)), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					NameRegex: regexp.MustCompile(".*admin.*"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "admin-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "hello-admin-pod-2",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
				},
			},
		},
		{
			name: "List pods with min age",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0])), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					MinAge:    5 * time.Minute,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "old-pod",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
				},
			},
		},
		{
			name: "List pods with max age",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().
					PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[2:]...)), gomock.Any()).
					Return(nil).
					Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					MaxAge:    5 * time.Minute,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "old-pod",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "very-old-pod",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * time.Minute)),
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "recent-pod",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
		},
		{
			name: "List pods with specific status",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0])), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					PodStatus: v1.PodRunning,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "running-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pending-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
				},
			},
		},
		{
			name: "Delete matching pods with confirmation",
			prepare: func(_ *testing.T, _ *fields, s *shared) error {
				s.in.Write([]byte("y\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionDelete,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-2",
							Namespace: "default2",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					podList, err := f.clientSet.CoreV1().Pods("default").List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)
					assert.Empty(t, podList.Items, "Expected no pods in default namespace after deletion")

					podList2, err := f.clientSet.CoreV1().Pods("default2").List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)
					assert.Len(t, podList2.Items, 1, "Expected pods in default2 namespace after deletion")

					outBytes, err := io.ReadAll(s.out)
					require.NoError(t, err)
					errOutBytes, err := io.ReadAll(s.errOut)
					require.NoError(t, err)

					outStr := string(outBytes)
					errOutStr := string(errOutBytes)

					assert.Contains(t, outStr, "Deleted pod test-pod in namespace default")
					assert.Contains(t, errOutStr, "The following pods will be deleted:")
					assert.Contains(t, errOutStr, "- test-pod in namespace default")
					assert.Contains(t, errOutStr, "Are you sure you want to continue? [y/N]: ")
				},
			},
		},
		{
			name: "Delete matching pods cancelled",
			prepare: func(_ *testing.T, _ *fields, s *shared) error {
				s.in.Write([]byte("n\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionDelete,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, _ *shared) {
					podList, err := f.clientSet.CoreV1().Pods("default").List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)
					assert.Len(t, podList.Items, 1, "Expected 1 pod in default namespace after cancellation")
				},
			},
		},
		{
			name: "Force delete matching pods skips graceful deletion",
			prepare: func(_ *testing.T, _ *fields, s *shared) error {
				s.in.Write([]byte("y\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionDelete,
					Force:     true,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					podList, err := f.clientSet.CoreV1().Pods("default").List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)
					assert.Empty(t, podList.Items, "Expected no pods in default namespace after force deletion")

					outBytes, err := io.ReadAll(s.out)
					require.NoError(t, err)
					outStr := string(outBytes)
					assert.Contains(t, outStr, "Deleted pod test-pod in namespace default")
				},
			},
		},
		{
			name: "Patch matching pods",
			prepare: func(_ *testing.T, _ *fields, s *shared) error {
				s.in.Write([]byte("y\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionPatch,
					Patch:     `{"metadata":{"labels":{"patched":"true"}}}`,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					podList, err := f.clientSet.CoreV1().Pods("default").List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)
					assert.Len(t, podList.Items, 1, "Expected 1 pod in default namespace after patching")
					assert.Equal(
						t,
						"true",
						podList.Items[0].Labels["patched"],
						"Expected pod to be patched with label 'patched=true'",
					)

					outBytes, err := io.ReadAll(s.out)
					require.NoError(t, err)
					errOutBytes, err := io.ReadAll(s.errOut)
					require.NoError(t, err)

					outStr := string(outBytes)
					errOutStr := string(errOutBytes)

					assert.Contains(t, outStr, "Patched pod test-pod in namespace default")
					assert.Contains(t, errOutStr, "The following pods will be patched:")
					assert.Contains(t, errOutStr, "- test-pod in namespace default")
					assert.Contains(t, errOutStr, "Are you sure you want to continue? [y/N]: ")
				},
			},
		},
		{
			name: "List pods with node name regex",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().
					PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0:2]...)), gomock.Any()).
					Return(nil).
					Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:     "default",
					Action:        ActionList,
					NodeNameRegex: regexp.MustCompile("node"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
						Spec: v1.PodSpec{
							NodeName: "node-1",
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-2",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase:             v1.PodPending,
							NominatedNodeName: "node-1",
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-3",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
						Spec: v1.PodSpec{
							NodeName: "other-2",
						},
					},
				},
			},
		},
		{
			name: "List pods that have been restarted",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().
					PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0])), gomock.Any()).
					Return(nil).
					Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					Restarted: true,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
							ContainerStatuses: []v1.ContainerStatus{
								{
									Name:         "test-container",
									Ready:        true,
									RestartCount: 1,
								},
							},
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-2",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase:             v1.PodPending,
							NominatedNodeName: "node-1",
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-3",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
						Spec: v1.PodSpec{
							NodeName: "other-2",
						},
					},
				},
			},
		},
		{
			name: "List pods matching image regex",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().
					PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0:2]...)), gomock.Any()).
					Return(nil).
					Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:  "default",
					Action:     ActionList,
					ImageRegex: regexp.MustCompile("^bitnami/.*"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "test-container",
									Image: "bitnami/nginx",
								},
							},
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-2",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase:             v1.PodPending,
							NominatedNodeName: "node-1",
						},
						Spec: v1.PodSpec{
							InitContainers: []v1.Container{
								{
									Name:  "init-container",
									Image: "bitnami/shell",
								},
							},
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-3",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
						Spec: v1.PodSpec{
							NodeName: "other-2",
						},
					},
				},
			},
		},
		{
			name: "List pods with jq filter",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().
					PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0:2]...)), gomock.Any()).
					Return(nil).
					Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace: "default",
					Action:    ActionList,
					JQQuery: func() *gojq.Query {
						q, err := pkg.PrepareQuery(".status.phase == \"Running\"")
						require.NoError(t, err)
						return q
					}(),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "running-pod-1",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "running-pod-2",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodRunning,
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pending-pod",
							Namespace: "default",
						},
						Status: v1.PodStatus{
							Phase: v1.PodPending,
						},
					},
				},
			},
		},
	}

	test := func(prepare func(*testing.T, *fields, *shared) error, args args, shared shared, want want) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			for _, resource := range shared.resources {
				if pod, ok := resource.(*v1.Pod); ok {
					// Set a default creation timestamp if not set
					if pod.CreationTimestamp.IsZero() {
						pod.CreationTimestamp = metav1.NewTime(time.Now())
					}
				}
			}

			ff := fields{}
			ff.clientSet = fake.NewClientset(shared.resources...)

			shared.streams, shared.in, shared.out, shared.errOut = genericclioptions.NewTestIOStreams()

			require.NoError(t, prepare(t, &ff, &shared))

			handler := PodHandler{
				clientSet:      ff.clientSet,
				printer:        ff.printer,
				executorGetter: ff.executorGetter,
			}

			args.options.Streams = &shared.streams
			err := handler.HandleAction(t.Context(), args.options)
			if want.err != nil {
				require.Error(t, err)
				require.EqualError(t, err, want.err.Error())
			} else {
				require.NoError(t, err)
			}

			if want.check != nil {
				want.check(t, &ff, &shared)
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, test(tt.prepare, tt.args, tt.shared, tt.want))
	}
}
