package types

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/alikhil/kubectl-find/pkg/mocks"
	"github.com/alikhil/kubectl-find/pkg/printers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	k8s_types "k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestUniversalHandler(t *testing.T) {
	type shared struct {
		resources []runtime.Object
		in        *bytes.Buffer
		out       *bytes.Buffer
		errOut    *bytes.Buffer
		streams   genericclioptions.IOStreams
	}

	type fields struct {
		printer printers.BatchPrinter
		client  dynamic.Interface
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
			name: "List configmaps",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))

				// Create a list of unstructured objects for the expected objects
				expectedItems := toUL(t, s.resources...)
				m.EXPECT().PrintObjects(gomock.InAnyOrder(expectedItems), gomock.Any()).Return(nil).Times(1)

				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionList,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm",
							Namespace: "default",
						},
					},
				},
			},
		},
		{
			name: "List secrets",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				expectedItems := toUL(t, s.resources...)
				m.EXPECT().PrintObjects(gomock.InAnyOrder(expectedItems), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionList,
					ResourceType: getResource("secret"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Secret{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Secret",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
		},
		{
			name: "List in empty cluster",
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionList,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{},
			},
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.Any(), gomock.Any()).Return(nil).Times(0)

				f.printer = m
				return nil
			},
		},
		{
			name: "List in empty namespace",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.Any(), gomock.Any()).Return(nil).Times(0)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "nonexistent",
					Action:       ActionList,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm",
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
					Namespace:    "",
					Action:       ActionList,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm-1",
							Namespace: "default",
						},
					},
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm-2",
							Namespace: "kube-system",
						},
					},
				},
			},
		},
		{
			name: "List resources matching regex",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0])), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionList,
					NameRegex:    regexp.MustCompile("test\\-cm\\-1"),
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm-1",
							Namespace: "default",
						},
					},
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm-2",
							Namespace: "default",
						},
					},
				},
			},
		},
		{
			name: "List resources with min age",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[0])), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionList,
					MinAge:       5 * time.Minute,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:              "old-cm",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						},
					},
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:              "new-cm",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now()),
						},
					},
				},
			},
		},
		{
			name: "List resources with max age",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				m.EXPECT().PrintObjects(gomock.InAnyOrder(toUL(t, s.resources[1])), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionList,
					MaxAge:       5 * time.Minute,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:              "old-cm",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
						},
					},
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:              "new-cm",
							Namespace:         "default",
							CreationTimestamp: metav1.NewTime(time.Now()),
						},
					},
				},
			},
		},
		{
			name: "Delete resource with confirmation",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				s.in.Write([]byte("y\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionDelete,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm",
							Namespace: "default",
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					outBytes, err := io.ReadAll(s.out)
					assert.NoError(t, err)
					errOutBytes, err := io.ReadAll(s.errOut)
					assert.NoError(t, err)

					outStr := string(outBytes)
					errOutStr := string(errOutBytes)

					assert.Contains(t, outStr, "Deleted configmap test-cm")
					assert.Contains(t, errOutStr, "The following configmaps will be deleted:")
					assert.Contains(t, errOutStr, "- test-cm")
				},
			},
		},
		{
			name: "Delete resource cancelled",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				s.in.Write([]byte("n\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionDelete,
					ResourceType: getResource("configmap"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm",
							Namespace: "default",
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					errOutBytes, err := io.ReadAll(s.errOut)
					assert.NoError(t, err)
					errOutStr := string(errOutBytes)
					assert.Contains(t, errOutStr, "Deletion cancelled.")
				},
			},
		},
		{
			name: "Patch resource with confirmation",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				s.in.Write([]byte("y\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionPatch,
					ResourceType: getResource("configmap"),
					Patch:        `{"metadata":{"labels":{"patched":"true"}}}`,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm",
							Namespace: "default",
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					outBytes, err := io.ReadAll(s.out)
					assert.NoError(t, err)
					errOutBytes, err := io.ReadAll(s.errOut)
					assert.NoError(t, err)

					outStr := string(outBytes)
					errOutStr := string(errOutBytes)

					assert.Contains(t, outStr, "Patched configmap test-cm")
					assert.Contains(t, errOutStr, "The following configmaps will be patched:")
					assert.Contains(t, errOutStr, "- test-cm")
				},
			},
		},
		{
			name: "Patch resource cancelled",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				s.in.Write([]byte("n\n"))
				return nil
			},
			args: args{
				options: ActionOptions{
					Namespace:    "default",
					Action:       ActionPatch,
					ResourceType: getResource("configmap"),
					Patch:        `{"metadata":{"labels":{"patched":"true"}}}`,
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cm",
							Namespace: "default",
						},
					},
				},
			},
			want: want{
				check: func(t *testing.T, f *fields, s *shared) {
					errOutBytes, err := io.ReadAll(s.errOut)
					assert.NoError(t, err)
					errOutStr := string(errOutBytes)
					assert.Contains(t, errOutStr, "Patch cancelled.")
				},
			},
		},
		{
			name: "List non-namespaced resources",
			prepare: func(t *testing.T, f *fields, s *shared) error {
				m := mocks.NewMockBatchPrinter(gomock.NewController(t))
				expectedItems := toUL(t, s.resources...)
				m.EXPECT().PrintObjects(gomock.InAnyOrder(expectedItems), gomock.Any()).Return(nil).Times(1)
				f.printer = m
				return nil
			},
			args: args{
				options: ActionOptions{
					Action:       ActionList,
					ResourceType: getResource("namespace"),
				},
			},
			shared: shared{
				resources: []runtime.Object{
					&v1.Namespace{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Namespace",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: "default",
						},
					},
					&v1.Namespace{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Namespace",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: "kube-system",
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
				// Set creation timestamps for resources if not already set
				if cm, ok := resource.(*v1.ConfigMap); ok {
					if cm.CreationTimestamp.IsZero() {
						cm.CreationTimestamp = metav1.NewTime(time.Now())
					}
				} else if secret, ok := resource.(*v1.Secret); ok {
					if secret.CreationTimestamp.IsZero() {
						secret.CreationTimestamp = metav1.NewTime(time.Now())
					}
				} else if ns, ok := resource.(*v1.Namespace); ok {
					if ns.CreationTimestamp.IsZero() {
						ns.CreationTimestamp = metav1.NewTime(time.Now())
					}
				}
			}

			ff := fields{}

			// Create a fake dynamic client with pre-populated resources
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))

			ff.client = dynamicfake.NewSimpleDynamicClient(scheme, shared.resources...)

			shared.streams, shared.in, shared.out, shared.errOut = genericclioptions.NewTestIOStreams()

			require.NoError(t, prepare(t, &ff, &shared))

			handler := UniversalHandler{
				opts: UniversalHandlerOptions{
					Printer:  ff.printer,
					Client:   ff.client,
					Resource: args.options.ResourceType,
				},
			}

			args.options.Streams = &shared.streams
			// since strategic merge patch does not work with unstructured objects in fake client, we use merge patch
			args.options.PatchStrategy = k8s_types.MergePatchType
			err := handler.HandleAction(context.Background(), args.options)
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
		tt := tt
		t.Run(tt.name, test(tt.prepare, tt.args, tt.shared, tt.want))
	}
}
