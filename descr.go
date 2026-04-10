package ask

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// cmdDescription is a command with fully parsed flags,
// used during Run to interact with the command.
type cmdDescription struct {
	Cmd Command

	// Name of the command itself
	Name string

	Root          *FlagGroup
	RemainingArgs []string

	// SeenFlags are the flag paths of the flags that were set.
	SeenFlags map[string]struct{}

	Config *runConfig
}

func loadCmdDescription(cmd Command, cfg *runConfig) (*cmdDescription, error) {
	val := reflect.ValueOf(cmd)
	grp := new(FlagGroup)
	grp.GroupName = ""

	// Load the flag definitions
	err := grp.Load(val)
	if err != nil {
		return nil, fmt.Errorf("cannot load flags: %w", err)
	}
	name := InferName(cmd)
	// Create the description of this command, with its flags
	out := &cmdDescription{
		Cmd:       cmd,
		Name:      name,
		Root:      grp,
		SeenFlags: make(map[string]struct{}),
		Config:    cfg,
	}
	return out, nil
}

func (descr *cmdDescription) applyArgs(ctx context.Context, args []string) error {
	// We don't apply args to sub-commands upfront;
	// sub-commands are considered only when and if needed,
	// and executed as a nested Run call.

	set := func(fl PrefixedFlag, value string) error {
		descr.SeenFlags[fl.Path] = struct{}{}

		if fl.Deprecated != "" && descr.Config.OnDeprecated != nil {
			if err := descr.Config.OnDeprecated(ctx, fl); err != nil {
				return err
			}
		}

		if err := fl.Flag.Value.Set(value); err != nil {
			return fmt.Errorf("failed to set flag %q: %w", fl.Path, err)
		}
		return nil
	}

	allFlags := descr.Root.All("")

	// First try to set all flags from env vars.
	// These flag changes may be overridden later by arg based flags.
	envFn := EnvFnFromContext(ctx)
	for _, pf := range allFlags {
		if pf.IsArg {
			continue
		}
		envKey := pf.Env
		// skip if explicitly set to ignore
		if envKey == "-" {
			continue
		}
		// infer env var name, if not set
		if envKey == "" {
			envKey = FlagPathToEnvKey(pf.Path)
		}
		// Lookup env var. If not set, ignore
		if v, ok := envFn(envKey); ok {
			if err := set(pf, v); err != nil {
				return fmt.Errorf("cannot set flag %q from env var %q: %w", pf.Path, envKey, err)
			}
		}
	}

	// Collect remaining flags to set.
	var long []PrefixedFlag
	var short []PrefixedFlag
	var positionalRequired []PrefixedFlag
	var positionalOptional []PrefixedFlag
	for _, pf := range allFlags {
		if pf.IsArg {
			if pf.Required {
				positionalRequired = append(positionalRequired, pf)
			} else {
				positionalOptional = append(positionalOptional, pf)
			}
		} else {
			if pf.Shorthand != 0 {
				short = append(short, pf)
			}
			if string(pf.Shorthand) != pf.Name {
				long = append(long, pf)
			}
		}
	}
	sort.SliceStable(long, func(i, j int) bool {
		return long[i].Path < long[j].Path
	})
	sort.SliceStable(short, func(i, j int) bool {
		return short[i].Path < short[j].Path
	})

	remaining, err := ParseArgs(short, long, args, set)
	if err != nil {
		// can be a HelpErr to indicate a help-flag was detected
		return err
	}

	var remainingPositionalRequiredFlags []PrefixedFlag
	for _, v := range positionalRequired {
		if _, ok := descr.SeenFlags[v.Path]; !ok {
			remainingPositionalRequiredFlags = append(remainingPositionalRequiredFlags, v)
		}
	}
	var remainingPositionalOptionalFlags []PrefixedFlag
	for _, v := range positionalOptional {
		if _, ok := descr.SeenFlags[v.Path]; !ok {
			remainingPositionalOptionalFlags = append(remainingPositionalOptionalFlags, v)
		}
	}

	// process required args
	if len(remaining) < len(remainingPositionalRequiredFlags) {
		remainingPaths := make([]string, 0, len(remainingPositionalRequiredFlags))
		for _, pf := range remainingPositionalRequiredFlags {
			remainingPaths = append(remainingPaths, pf.Path)
		}
		return fmt.Errorf("got %d arguments, but expected %d, missing required arguments: %s",
			len(remaining), len(remainingPositionalRequiredFlags), strings.Join(remainingPaths, ", "))
	}
	for i := range remainingPositionalRequiredFlags {
		if err := set(remainingPositionalRequiredFlags[i], remaining[i]); err != nil {
			return err
		}
	}
	remaining = remaining[len(remainingPositionalRequiredFlags):]

	// process optional args
	if len(remainingPositionalOptionalFlags) > 0 {
		count := 0
		for i := range remaining {
			if i >= len(remainingPositionalOptionalFlags) {
				break
			}
			if err := set(remainingPositionalOptionalFlags[i], remaining[i]); err != nil {
				return err
			}
			count += 1
		}
		remaining = remaining[count:]
	}

	descr.RemainingArgs = remaining

	return nil
}

func (descr *cmdDescription) String() string {
	return descr.Name
}

func (descr *cmdDescription) Command() Command {
	return descr.Cmd
}
