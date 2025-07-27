/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/alikhil/kubectl-find/pkg/cmd"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

// Using the defaults from goreleaser as per https://goreleaser.com/cookbooks/using-main.version/
//
//nolint:gochecknoglobals
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-find", pflag.ExitOnError)
	//nolint:reassign // flags are shared
	pflag.CommandLine = flags

	root := cmd.NewCmdFind(genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number of kubectl-find",
		Run: func(_ *cobra.Command, _ []string) {
			//nolint:forbidigo // this is a CLI tool, so printing version info is acceptable
			fmt.Printf("version: %s\ncommit: %s\ndate: %s\n", version, commit, date)
			os.Exit(0)
		},
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
