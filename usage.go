package ask

import (
	"errors"
	"fmt"
	"strings"
)

// InferName infers the name of a command.
// If the Command is Named, that is the name.
// If not, the name is the type name.
func InferName(cmd Command) string {
	if v, ok := cmd.(Named); ok {
		return v.Name()
	}
	return fmt.Sprintf("%T", cmd)
}

// SubCommandsUsage renders a short list of sub-commands and their help info,
// for a basic list of sub-commands in MoreHelp.
func SubCommandsUsage(routes ...Command) string {
	var out strings.Builder

	out.WriteString("Sub commands:\n")

	maxRouteLen := 0
	var names []string
	for _, subCmd := range routes {
		k := InferName(subCmd)
		if len(k) > maxRouteLen {
			maxRouteLen = len(k)
		}
		names = append(names, k)
	}

	for i, subCmd := range routes {
		k := names[i]
		out.WriteString("  ")
		out.WriteString(k)
		if len(k) < maxRouteLen {
			out.WriteString(strings.Repeat(" ", maxRouteLen-len(k)))
		}
		out.WriteString("  ")
		if h, ok := subCmd.(Help); ok {
			out.WriteString(h.Help())
		}
		out.WriteString("\n")
	}

	return out.String()
}

// UsageFromErr retrieves the usage information from an error,
// based on the failing command stack.
func UsageFromErr(err error) string {
	var v *cmdDescrError
	if !errors.As(err, &v) {
		return err.Error()
	}
	var out strings.Builder
	for i := range v.Stack {
		if i != 0 {
			out.WriteString(" ")
		}
		d := v.Stack[len(v.Stack)-1-i]
		out.WriteString(d.Name)
	}
	writeUsage(&out, v.Stack[0])
	return out.String()
}

func writeUsage(out *strings.Builder, descr *cmdDescription) {
	all := descr.Root.All("")

	for _, a := range all {
		if a.IsArg && a.Required {
			out.WriteString(" <")
			out.WriteString(a.Path)
			out.WriteString(">")
		}
	}
	for _, a := range all {
		if a.IsArg && !a.Required {
			out.WriteString(" [")
			out.WriteString(a.Path)
			out.WriteString("]")
		}
	}
	flagCount := 0
	for _, a := range all {
		if !a.IsArg && (!a.Hidden || descr.Config.ShowHidden) {
			flagCount += 1
		}
	}
	if flagCount > 0 {
		out.WriteString(fmt.Sprintf(" # %d flags (see below)", flagCount))
	}

	out.WriteString("\n\n")

	if len(all) > 0 {
		descr.Root.Usage("", descr.Config.ShowHidden, out)
		out.WriteString("\n")
	}

	if v, ok := descr.Cmd.(MoreHelp); ok {
		out.WriteString(v.MoreHelp())
		out.WriteString("\n")
	}
}
