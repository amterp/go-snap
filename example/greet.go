// Package example holds two toy systems and the snapshot suites that test
// them. They exist to be read: cli_test.go is the shape almost every suite
// starts as, and session_test.go is what that shape grows into when a case is
// a sequence of interactions rather than a single call.
package example

import (
	"fmt"
	"strconv"
	"strings"
)

// greeting is the toy CLI under test: it greets a name, with a couple of
// flags and a couple of ways to fail.
func greet(locale string, args []string, stdin string) (stdout, stderr string, exit int) {
	var out, errOut strings.Builder

	word, ok := map[string]string{"en": "Hello", "fr": "Bonjour", "de": "Hallo"}[locale]
	if !ok {
		fmt.Fprintf(&errOut, "greet: unknown locale %q\n", locale)
		return out.String(), errOut.String(), 2
	}

	var names []string
	shout := false
	repeat := 1
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--shout":
			shout = true
		case arg == "--repeat":
			i++
			if i >= len(args) {
				fmt.Fprintf(&errOut, "greet: --repeat needs a count\n")
				return out.String(), errOut.String(), 2
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				fmt.Fprintf(&errOut, "greet: --repeat wants a positive number, got %q\n", args[i])
				return out.String(), errOut.String(), 2
			}
			repeat = n
		case arg == "-":
			names = append(names, strings.Fields(stdin)...)
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(&errOut, "greet: unknown flag %s\n", arg)
			return out.String(), errOut.String(), 2
		default:
			names = append(names, arg)
		}
	}

	if len(names) == 0 {
		fmt.Fprintf(&errOut, "greet: no name given\n")
		return out.String(), errOut.String(), 2
	}

	for range repeat {
		for _, name := range names {
			line := fmt.Sprintf("%s, %s!", word, name)
			if shout {
				line = strings.ToUpper(line)
			}
			fmt.Fprintln(&out, line)
		}
	}
	return out.String(), errOut.String(), 0
}
