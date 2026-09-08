package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"
)

func TestNegatableStringValue(t *testing.T) {
	t.Parallel()

	var include, exclude string
	negated := false
	value := newNegatableStringValue(&include, &exclude, &negated)

	if err := value.Set("prod"); err != nil {
		t.Fatalf("set included value: %v", err)
	}
	negated = true
	if err := value.Set("test"); err != nil {
		t.Fatalf("set excluded value: %v", err)
	}
	if include != "prod" || exclude != "test" {
		t.Fatalf("got include=%q exclude=%q, want include=prod exclude=test", include, exclude)
	}

	if err := value.Set("canary"); err != nil {
		t.Fatalf("replace excluded value: %v", err)
	}
	if exclude != "canary" {
		t.Fatalf("got excluded value %q, want canary", exclude)
	}
}

func TestInitializeDiscoveryClientAndRESTMapper(t *testing.T) {
	t.Parallel()

	options := NewFindOptions(genericiooptions.IOStreams{})
	options.rest = &rest.Config{Host: "https://api.example.test"}

	if err := options.initializeDiscoveryClientAndRESTMapper(); err != nil {
		t.Fatalf("initialize discovery client and REST mapper: %v", err)
	}
	if options.discoveryClient == nil || options.resourceMapper == nil {
		t.Fatal("expected REST mapper to be initialized")
	}
}

func TestFindFlagParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, options *FindOptions)
	}{
		{
			name: "normal filters",
			args: []string{"-r", "prod", "-l", "app=nginx", "--host", "host-1", "--restarted"},
			check: func(t *testing.T, options *FindOptions) {
				t.Helper()
				if options.regex != "prod" || options.labelSelector != "app=nginx" ||
					options.nodeNameRegex != "host-1" ||
					!options.restarted {
					t.Fatalf("normal filters were not parsed: %+v", options)
				}
			},
		},
		{
			name: "negated filters",
			args: []string{
				"--not",
				"-r",
				"test",
				"-l",
				"version=1.20",
				"--status",
				"Failed",
				"--host",
				"host-2",
				"--image",
				"debug",
				"--jq",
				".metadata.labels.test == \"true\"",
				"--restarted",
			},
			check: func(t *testing.T, options *FindOptions) {
				t.Helper()
				if options.excludedRegex != "test" || options.excludedLabelSelector != "version=1.20" ||
					options.excludedPodStatus != "Failed" ||
					options.excludedNodeNameRegex != "host-2" ||
					options.excludedImageRegex != "debug" ||
					options.excludedJQFilter != ".metadata.labels.test == \"true\"" ||
					!options.excludeRestarted {
					t.Fatalf("negated filters were not parsed: %+v", options)
				}
			},
		},
		{
			name: "combined filters",
			args: []string{
				"-r",
				"prod",
				"-l",
				"app=nginx",
				"--node",
				"host-1",
				"--not",
				"-r",
				"test",
				"-l",
				"version=1.20",
				"--host",
				"host-2",
				"--restarted",
			},
			check: func(t *testing.T, options *FindOptions) {
				t.Helper()
				if options.regex != "prod" || options.excludedRegex != "test" || options.labelSelector != "app=nginx" ||
					options.excludedLabelSelector != "version=1.20" ||
					options.nodeNameRegex != "host-1" ||
					options.excludedNodeNameRegex != "host-2" ||
					!options.excludeRestarted {
					t.Fatalf("combined filters were not parsed: %+v", options)
				}
			},
		},
		{
			name: "controller filters",
			args: []string{"--controller", "apps/Daemonsets", "--not", "--controller", "batch/jobs"},
			check: func(t *testing.T, options *FindOptions) {
				t.Helper()
				if options.controller != "apps/Daemonsets" || options.excludedController != "batch/jobs" {
					t.Fatalf("controller filters were not parsed: %+v", options)
				}
			},
		},
		{
			name: "negated namespace",
			args: []string{"--not", "-n", "kube-system"},
			check: func(t *testing.T, options *FindOptions) {
				t.Helper()
				if options.excludedNamespace != "kube-system" || options.namespaceSpecified {
					t.Fatalf("negated namespace was not parsed: %+v", options)
				}
			},
		},
		{
			name: "normal namespace before negated filter",
			args: []string{"-n", "production", "--not", "-r", "test"},
			check: func(t *testing.T, options *FindOptions) {
				t.Helper()
				if !options.namespaceSpecified || *options.configFlags.Namespace != "production" ||
					options.excludedRegex != "test" {
					t.Fatalf("combined namespace and negated filter were not parsed: %+v", options)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			options := NewFindOptions(genericiooptions.IOStreams{})
			command := newCmdFind(options)
			if err := command.ParseFlags(tt.args); err != nil {
				t.Fatalf("parse flags: %v", err)
			}
			tt.check(t, options)
		})
	}
}

func TestEveryFlagParses(t *testing.T) {
	t.Parallel()

	options := NewFindOptions(genericiooptions.IOStreams{})
	command := newCmdFind(options)
	args := []string{
		"--all-namespaces", "--annotate", "owner=platform", "--annotations", "owner,team",
		"--as", "alice", "--as-group", "developers", "--as-uid", "1000", "--as-user-extra", "scope=read",
		"--cache-dir", "/tmp/kubectl-find-cache", "--certificate-authority", "/tmp/ca.crt",
		"--client-certificate", "/tmp/client.crt", "--client-key", "/tmp/client.key", "--cluster", "cluster-a",
		"--drain-chunk-size", "500", "--context", "context-a", "--cordon", "--delete", "--drain-delete-emptydir-data",
		"--disable-compression", "--drain-disable-eviction", "--drain", "--exec", "echo ok", "--force", "--drain-grace-period", "30",
		"--controller", "apps/daemonsets", "--host", "host-1", "--image", "nginx", "--insecure-skip-tls-verify", "--jq", ".metadata.name != null",
		"--drain-ignore-daemonsets", "--kubeconfig", "/tmp/config", "--labels", "app,version", "--max-age", "24h", "--min-age", "1h",
		"--name", "prod", "--namespace", "production", "--natural-sort", "--node", "host-2",
		"--node-condition", "Ready=True", "--node-labels", "topology.kubernetes.io/zone", "--patch", `{"metadata":{}}`,
		"--drain-pod-selector", "app=nginx", "--proxy-url", "http://proxy.example.test", "--request-timeout", "5s", "--restarted", "--selector", "app=nginx", "--server", "https://api.example.test",
		"--skip-confirm", "--status", "Running", "--tls-server-name", "api.example.test", "--token", "token",
		"--drain-skip-wait-for-delete-timeout", "60", "--drain-timeout", "2m", "--uncordon", "--user", "user-a", "--not",
	}
	if err := command.ParseFlags(args); err != nil {
		t.Fatalf("parse every flag: %v", err)
	}
	if err := command.ParseFlags([]string{"--help"}); !errors.Is(err, pflag.ErrHelp) {
		t.Fatalf("parse help flag: got %v, want pflag.ErrHelp", err)
	}

	expected := map[string]bool{
		"all-namespaces": true, "annotate": true, "annotations": true, "as": true, "as-group": true,
		"as-uid": true, "as-user-extra": true, "cache-dir": true, "certificate-authority": true,
		"drain-chunk-size": true, "client-certificate": true, "client-key": true, "cluster": true, "context": true, "cordon": true,
		"delete": true, "drain-delete-emptydir-data": true, "disable-compression": true, "drain-disable-eviction": true, "drain": true,
		"controller": true, "exec": true, "force": true, "drain-grace-period": true, "host": true, "image": true, "drain-ignore-daemonsets": true,
		"insecure-skip-tls-verify": true, "jq": true, "kubeconfig": true, "labels": true, "max-age": true,
		"min-age": true, "name": true, "namespace": true, "natural-sort": true, "node": true,
		"node-condition": true, "node-labels": true, "not": true, "patch": true, "drain-pod-selector": true, "proxy-url": true, "request-timeout": true,
		"restarted": true, "selector": true, "server": true, "skip-confirm": true, "status": true,
		"drain-skip-wait-for-delete-timeout": true, "drain-timeout": true, "tls-server-name": true, "token": true, "uncordon": true, "user": true,
	}
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		if !expected[flag.Name] {
			t.Errorf("flag %q is missing from parsing coverage", flag.Name)
			return
		}
		if !flag.Changed {
			t.Errorf("flag %q was not parsed", flag.Name)
		}
		delete(expected, flag.Name)
	})
	for name := range expected {
		t.Errorf("expected flag %q was not registered", name)
	}
}
