package e2e_test

// import (
// 	"bytes"
// 	"context"
// 	"os"
// 	"testing"

// 	"github.com/alikhil/kubectl-find/pkg/cmd"
// 	"github.com/stretchr/testify/assert"
// 	"k8s.io/cli-runtime/pkg/genericiooptions"
// 	"sigs.k8s.io/e2e-framework/pkg/env"
// 	"sigs.k8s.io/e2e-framework/pkg/envconf"
// 	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
// 	"sigs.k8s.io/e2e-framework/pkg/features"
// 	"sigs.k8s.io/e2e-framework/support/kind"
// )

// var testenv env.Environment

// func TestMain(m *testing.M) {
// 	// if env vars USE_EXISTING_CLUSTER and KIND_CLUSTER_NAME are set, use them
// 	var kindClusterName string
// 	var namespace string

// 	if os.Getenv("USE_EXISTING_CLUSTER") == "true" {
// 		kindClusterName = os.Getenv("KIND_CLUSTER_NAME")
// 		namespace = os.Getenv("KIND_NAMESPACE")
// 	}
// 	if kindClusterName == "" {
// 		kindClusterName = envconf.RandomName("my-cluster", 16)
// 	}
// 	if namespace == "" {
// 		namespace = envconf.RandomName("myns", 16)
// 	}

// 	testenv = env.New()

// 	initializers := []env.Func{
// 		envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
// 		envfuncs.CreateNamespace(namespace),
// 	}
// 	// Use pre-defined environment funcs to create a kind cluster prior to test run
// 	testenv.Setup(initializers...)

// 	finalizers := []env.Func{
// 		envfuncs.DeleteNamespace(namespace),
// 	}
// 	if os.Getenv("USE_EXISTING_CLUSTER") != "true" {
// 		finalizers = append(finalizers,
// 			envfuncs.DestroyCluster(kindClusterName),
// 		)
// 	}
// 	// Use pre-defined environment funcs to teardown kind cluster after tests
// 	testenv.Finish(
// 		finalizers...,
// 	)

// 	// launch package tests
// 	os.Exit(testenv.Run(m))
// }

// func TestKubernetes(t *testing.T) {
// 	f1 := features.New("count pod").
// 		WithLabel("type", "pod-count").
// 		Assess("pods from kube-system", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
// 			// var pods corev1.PodList
// 			cfgFile := cfg.KubeconfigFile()
// 			os.Setenv("KUBECONFIG", cfgFile)
// 			buf := &bytes.Buffer{}

// 			root := cmd.NewCmdFind(genericiooptions.IOStreams{In: os.Stdin, Out: buf, ErrOut: os.Stderr})
// 			root.SetArgs([]string{"-n", "kube-system", "--type", "pods"})

// 			err := root.Execute()
// 			if err != nil {
// 				t.Fatalf("failed to execute command: %v", err)
// 			}

// 			stdoutRes := buf.String()
// 			expectedRes := "NAME   STATUS   AGE\n"
// 			assert.Equal(t, expectedRes, stdoutRes, "stdout should match expected output")

// 			t.Logf("Command output: %s", buf.String())

// 			// err := cfg.Client().Resources("kube-system").List(context.TODO(), &pods)
// 			// if err != nil {
// 			// 	t.Fatal(err)
// 			// }
// 			// if len(pods.Items) == 0 {
// 			// 	t.Fatal("no pods in namespace kube-system")
// 			// }
// 			return ctx
// 		}).Feature()

// 	// f2 := features.New("count namespaces").
// 	// 	WithLabel("type", "ns-count").
// 	// 	Assess("namespace exist", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
// 	// 		var nspaces corev1.NamespaceList
// 	// 		err := cfg.Client().Resources().List(context.TODO(), &nspaces)
// 	// 		if err != nil {
// 	// 			t.Fatal(err)
// 	// 		}
// 	// 		if len(nspaces.Items) == 1 {
// 	// 			t.Fatal("no other namespace")
// 	// 		}
// 	// 		return ctx
// 	// 	}).Feature()

// 	// test feature
// 	testenv.Test(t, f1)
// }
