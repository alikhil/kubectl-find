package prompts

import (
	"bufio"
	"fmt"
	"strings"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func AskForConfirmation(streams *genericiooptions.IOStreams) bool {
	fmt.Fprint(streams.ErrOut, "Are you sure you want to continue? [y/N]: ")
	reader := bufio.NewReader(streams.In)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}
