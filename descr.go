package ask

import (
	"context"
	"errors"
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
	if !val.IsValid() || (val.Kind() == reflect.Ptr && val.IsNil()) {
		return nil, errors.New("cannot load flags of nil command")
	}
	grp := new(FlagGroup)
	grp.GroupName = ""

	// Apply the defaults, and load the flag definitions
	err := grp.Load(val)
	if err != nil {
		return nil, fmt.Errorf("cannot load flags: %w", err)
	}
	if err := checkFlagCollisions(grp.All("")); err != nil {
		return nil, err
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

	// First try to set all flags (including positional args) from env vars.
	// These flag changes may be overridden later by arg based flags.
	envFn := EnvFnFromContext(ctx)
	for _, pf := range allFlags {
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
	// Positional args can also be set as named flag, and are then not loaded positionally.
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
		}
		if pf.Shorthand != 0 {
			short = append(short, pf)
		}
		if string(pf.Shorthand) != pf.Name {
			long = append(long, pf)
		}
	}
	sort.SliceStable(long, func(i, j int) bool {
		return long[i].Path < long[j].Path
	})
	// The parser searches shorthands by shorthand, not by path
	sort.SliceStable(short, func(i, j int) bool {
		return short[i].Shorthand < short[j].Shorthand
	})

	remaining, err := ParseArgs(short, long, args, set)
	if err != nil {
		// can be a HelpErr to indicate a help-flag was detected
		return err
	}

	// Positional args that were already set (by env var or named flag) are skipped:
	// the remaining command-line arguments fill the other positional args
	// in declaration order, required args first, then optional args.
	positionalRequired = descr.unseen(positionalRequired)
	positionalOptional = descr.unseen(positionalOptional)

	// process required args
	if len(remaining) < len(positionalRequired) {
		missing := make([]string, 0, len(positionalRequired))
		for _, pf := range positionalRequired[len(remaining):] {
			missing = append(missing, pf.Path)
		}
		return fmt.Errorf("got %d arguments, but expected %d, missing required arguments: %s",
			len(remaining), len(positionalRequired), strings.Join(missing, ", "))
	}
	for i := range positionalRequired {
		if err := set(positionalRequired[i], remaining[i]); err != nil {
			return err
		}
	}
	remaining = remaining[len(positionalRequired):]

	// process optional args
	count := 0
	for i := range remaining {
		if i >= len(positionalOptional) {
			break
		}
		if err := set(positionalOptional[i], remaining[i]); err != nil {
			return err
		}
		count += 1
	}
	remaining = remaining[count:]

	descr.RemainingArgs = remaining

	return nil
}

// unseen filters out the flags that have already been set.
func (descr *cmdDescription) unseen(flags []PrefixedFlag) []PrefixedFlag {
	out := make([]PrefixedFlag, 0, len(flags))
	for _, pf := range flags {
		if _, ok := descr.SeenFlags[pf.Path]; !ok {
			out = append(out, pf)
		}
	}
	return out
}

// checkFlagCollisions returns an error if any two flags share a path,
// or any two flags share a shorthand, since the parser would only ever find one of them.
func checkFlagCollisions(all []PrefixedFlag) error {
	paths := make(map[string]struct{}, len(all))
	shorthands := make(map[uint8]string, len(all))
	for _, pf := range all {
		if _, ok := paths[pf.Path]; ok {
			return fmt.Errorf("flag/arg %q is declared more than once", pf.Path)
		}
		paths[pf.Path] = struct{}{}
		if pf.Shorthand == 0 {
			continue
		}
		if other, ok := shorthands[pf.Shorthand]; ok {
			return fmt.Errorf("flags %q and %q share shorthand -%s", other, pf.Path, string(pf.Shorthand))
		}
		shorthands[pf.Shorthand] = pf.Path
	}
	return nil
}

func (descr *cmdDescription) String() string {
	return descr.Name
}

func (descr *cmdDescription) Command() Command {
	return descr.Cmd
}
