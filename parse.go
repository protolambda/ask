package ask

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ApplyArg func(fl PrefixedFlag, value string) error

// ParseArgs parses arguments as flags (long and short format).
// Not all arguments may be consumed as flags, the remaining arguments are returned.
// Unrecognized flags result in an error.
// A HelpErr is returned if a flag like `--help` or `-h` is detected.
func ParseArgs(sortedShort []PrefixedFlag, sortedLong []PrefixedFlag,
	args []string, set ApplyArg) (remaining []string, err error) {
	for len(args) > 0 {
		s := args[0]
		args = args[1:]
		if len(s) == 0 || s[0] != '-' || len(s) == 1 {
			remaining = append(remaining, s)
			continue
		}

		if s[1] == '-' {
			if len(s) == 2 { // "--" terminates the flags
				remaining = append(remaining, args...)
				break
			}
			args, err = ParseLongArg(sortedLong, s, args, set)
		} else {
			args, err = ParseShortArg(sortedShort, s, args, set)
		}
		if err != nil {
			return
		}
	}
	return
}

// ParseLongArg parses an argument as long-flag.
// It may consume more arguments: remaining arguments to parse next are returned.
// A HelpErr is returned when a flag is detected like `--help`.
//
// The sortedFlags slice is ordered from low to high long string.
func ParseLongArg(sortedFlags []PrefixedFlag, firstArg string, args []string, fn ApplyArg) (nextArgs []string, err error) {
	nextArgs = args
	if len(firstArg) < 2 {
		return nil, fmt.Errorf("long-format flag to short: %q", firstArg)
	}
	name := firstArg[2:]
	if len(name) == 0 || name[0] == '-' || name[0] == '=' {
		return nil, fmt.Errorf("bad flag syntax: %s", firstArg)
	}

	split := strings.SplitN(name, "=", 2)
	name = split[0]

	flagIndex := sort.Search(len(sortedFlags), func(i int) bool {
		return sortedFlags[i].Path >= name
	})

	if flagIndex == len(sortedFlags) || sortedFlags[flagIndex].Path != name {
		// unrecognized
		if name == "help" {
			return nextArgs, HelpErr
		} else {
			return nextArgs, fmt.Errorf("unrecognized flag: %s", name)
		}
	}

	fl := sortedFlags[flagIndex]

	var value string
	if len(split) == 2 {
		// '--flag=arg'
		value = split[1]
	} else if flv, ok := fl.Value.(ImplicitValue); ok {
		// '--flag' (arg was optional)
		value = flv.Implicit()
	} else if len(nextArgs) > 0 {
		// '--flag arg'
		value = nextArgs[0]
		nextArgs = nextArgs[1:]
	} else {
		// '--flag' (arg was required)
		return nextArgs, fmt.Errorf("flag needs an argument: %s", firstArg)
	}

	if err := fn(fl, value); err != nil {
		return nextArgs, fmt.Errorf("failed to apply flag %s: %q, err: %v", name, value, err)
	}

	return nextArgs, nil
}

// sortedFlags is ordered from low to high shorthand string
func parseSingleShortArg(sortedFlags []PrefixedFlag, shorthands string, args []string, fn ApplyArg) (remainingShorthands string, nextArgs []string, err error) {
	if len(shorthands) == 0 {
		return "", nil, errors.New("no shorthand flags to parse")
	}

	nextArgs = args
	remainingShorthands = shorthands[1:]
	c := shorthands[0]

	flagIndex := sort.Search(len(sortedFlags), func(i int) bool {
		return sortedFlags[i].Shorthand < c
	})

	if flagIndex == len(sortedFlags) {
		switch {
		case c == 'h':
			return "", nil, HelpErr
		default:
			return "", nil, fmt.Errorf("unknown shorthand flag: %q in -%s", c, shorthands)
		}
	}

	fl := sortedFlags[flagIndex]

	var value string
	if len(shorthands) > 2 && shorthands[1] == '=' {
		// '-f=arg'
		value = shorthands[2:]
		remainingShorthands = ""
	} else if flv, ok := fl.Value.(ImplicitValue); ok {
		// '-f' (arg was optional)
		value = flv.Implicit()
	} else if len(shorthands) > 1 {
		// '-farg'
		value = shorthands[1:]
		remainingShorthands = ""
	} else if len(args) > 0 {
		// '-f arg'
		value = args[0]
		nextArgs = args[1:]
	} else {
		// '-f' (arg was required)
		return "", nil, fmt.Errorf("flag needs an argument: %q in -%s", c, shorthands)
	}

	if err := fn(fl, value); err != nil {
		return "", nil, fmt.Errorf("failed to apply flag %s: %v", string(c), value)
	}

	return remainingShorthands, nextArgs, nil
}

// ParseShortArg parses an argument as shorthand(s) string.
// It may consume more arguments: remaining arguments to parse next are returned.
// A HelpErr is returned when a flag is detected like `-h`.
//
// The sortedFlags slice is ordered from low to high shorthand string
func ParseShortArg(sortedFlags []PrefixedFlag, firstArg string, args []string, fn ApplyArg) (nextArgs []string, err error) {
	if len(firstArg) == 0 {
		return nil, errors.New("no shorthand flags to parse")
	}

	nextArgs = args
	shorthands := firstArg[1:]

	// "shorthands" can be a series of shorthand letters of flags (e.g. "-vvv").
	for len(shorthands) > 0 {
		shorthands, nextArgs, err = parseSingleShortArg(sortedFlags, shorthands, args, fn)
		if err != nil {
			return
		}
	}

	return
}
