// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Command is one node in the aeon command tree.
type Command struct {
	Name     string
	Short    string
	Long     string
	Use      string
	minArgs  int
	maxArgs  int
	addFlags func(*flagSet)
	run      func(args []string) error
	subs     []*Command
}

type flagKind int

const (
	flagBool flagKind = iota
	flagString
	flagInt
	flagStrings
)

type flagDef struct {
	name  string
	short byte
	kind  flagKind
	usage string
	str   *string
	b     *bool
	n     *int
	ss    *[]string
}

type flagSet struct {
	flags []*flagDef
}

func (fs *flagSet) add(f *flagDef) {
	fs.flags = append(fs.flags, f)
}

func (fs *flagSet) string(dest *string, name string, short byte, usage string) {
	fs.add(&flagDef{name: name, short: short, kind: flagString, usage: usage, str: dest})
}

func (fs *flagSet) bool(dest *bool, name string, short byte, usage string) {
	fs.add(&flagDef{name: name, short: short, kind: flagBool, usage: usage, b: dest})
}

func (fs *flagSet) int(dest *int, name string, usage string) {
	fs.add(&flagDef{name: name, kind: flagInt, usage: usage, n: dest})
}

func (fs *flagSet) strings(dest *[]string, name string, usage string) {
	fs.add(&flagDef{name: name, kind: flagStrings, usage: usage, ss: dest})
}

func (fs *flagSet) lookup(name string) *flagDef {
	for i := len(fs.flags) - 1; i >= 0; i-- {
		f := fs.flags[i]
		if f.name == name || (len(name) == 1 && f.short == name[0]) {
			return f
		}
	}
	return nil
}

func (fs *flagSet) parse(args []string) ([]string, error) {
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		name, value, hasValue, attached := splitFlag(a)
		def := fs.lookup(name)
		if def == nil {
			return nil, fmt.Errorf("unknown flag %s", flagLabel(a))
		}
		if def.kind == flagBool {
			if attached {
				return nil, fmt.Errorf("unknown flag %s", flagLabel(a))
			}
			if hasValue {
				v, err := strconv.ParseBool(value)
				if err != nil {
					return nil, fmt.Errorf("flag --%s: %w", def.name, err)
				}
				*def.b = v
			} else {
				*def.b = true
			}
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag --%s needs a value", def.name)
			}
			i++
			value = args[i]
		}
		switch def.kind {
		case flagString:
			*def.str = value
		case flagInt:
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("flag --%s: expected an integer", def.name)
			}
			*def.n = n
		case flagStrings:
			*def.ss = append(*def.ss, value)
		}
	}
	return pos, nil
}

func splitFlag(a string) (name, value string, hasValue, attached bool) {
	if strings.HasPrefix(a, "--") {
		body := strings.TrimPrefix(a, "--")
		if before, after, ok := strings.Cut(body, "="); ok {
			return before, after, true, false
		}
		return body, "", false, false
	}
	body := strings.TrimPrefix(a, "-")
	if body == "" {
		return "", "", false, false
	}
	if len(body) == 1 {
		return body, "", false, false
	}
	return body[:1], body[1:], true, true
}

func flagLabel(a string) string {
	if strings.HasPrefix(a, "--") {
		name, _, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		return "--" + name
	}
	return a
}

func (c *Command) child(name string) *Command {
	for _, s := range c.subs {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (rt *runtime) walk(cmd *Command, args []string) (*Command, []string, error) {
	var skipped []string
	for i := 0; i < len(args); {
		a := args[i]
		if a != "--" && strings.HasPrefix(a, "-") && a != "-" {
			n, ok, err := globalSkip(args, i)
			if err != nil {
				return cmd, nil, err
			}
			if ok {
				skipped = append(skipped, args[i:i+n]...)
				i += n
				continue
			}
			return cmd, append(skipped, args[i:]...), nil
		}
		if a != "--" {
			if sub := cmd.child(a); sub != nil {
				return rt.walk(sub, append(skipped, args[i+1:]...))
			}
		}
		return cmd, append(skipped, args[i:]...), nil
	}
	return cmd, skipped, nil
}

func globalSkip(args []string, i int) (int, bool, error) {
	a := args[i]
	name, _, hasValue, attached := splitFlag(a)
	takes, ok := globalFlag(name, attached)
	if !ok {
		return 0, false, nil
	}
	if !takes {
		return 1, true, nil
	}
	if hasValue {
		return 1, true, nil
	}
	if i+1 >= len(args) {
		label := "--" + name
		if name == "h" {
			label = "-h"
		}
		return 0, false, fmt.Errorf("flag %s needs a value", label)
	}
	return 2, true, nil
}

func globalFlag(name string, attached bool) (takesValue, ok bool) {
	switch name {
	case "help", "version", "json":
		if attached {
			return false, false
		}
		return false, true
	case "h":
		if attached {
			return false, false
		}
		return false, true
	case "config", "instance", "agent-name", "session-id":
		return true, true
	default:
		return false, false
	}
}

func (rt *runtime) bindGlobals(fs *flagSet) {
	defaultConfig := "~/.aeon/config.yaml"
	if rt.program == "paimos" {
		defaultConfig = "~/.paimos/config.yaml"
	}
	fs.string(&rt.configPath, "config", 0, "config file (default "+defaultConfig+")")
	fs.string(&rt.instance, "instance", 0, "named instance (default: default_instance)")
	fs.bool(&rt.jsonOut, "json", 0, "emit JSON")
	fs.string(&rt.agentName, "agent-name", 0, "agent name recorded on writes")
	fs.string(&rt.sessionID, "session-id", 0, "session id recorded on writes")
	fs.bool(&rt.help, "help", 'h', "show help")
	fs.bool(&rt.version, "version", 0, "print the calendar version")
}

func (rt *runtime) parse(cmd *Command, args []string) ([]string, error) {
	fs := &flagSet{}
	rt.bindGlobals(fs)
	if cmd.addFlags != nil {
		cmd.addFlags(fs)
	}
	pos, err := fs.parse(args)
	if err != nil {
		return nil, err
	}
	return pos, nil
}

func (rt *runtime) writeHelp(cmd *Command) {
	out := rt.stdout
	title := cmd.Short
	if title == "" {
		title = "Agents-first command line for PAIMOS AEON"
	}
	fmt.Fprintf(out, "%s — %s\n", rt.program, title)
	if cmd.Long != "" {
		fmt.Fprintf(out, "\n%s\n", cmd.Long)
	}
	use := cmd.Use
	if use == "" {
		use = "<command> [flags]"
	}
	fmt.Fprintf(out, "\nUsage:\n  %s %s\n", rt.program, use)
	if len(cmd.subs) > 0 {
		fmt.Fprintf(out, "\nCommands:\n")
		subs := append([]*Command(nil), cmd.subs...)
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name < subs[j].Name })
		for _, s := range subs {
			fmt.Fprintf(out, "  %-12s %s\n", s.Name, s.Short)
		}
	}
	fs := &flagSet{}
	if cmd.addFlags != nil {
		cmd.addFlags(fs)
	}
	if len(fs.flags) > 0 {
		fmt.Fprintf(out, "\nFlags:\n")
		writeFlagHelp(out, fs)
	}
	fmt.Fprintf(out, "\nGlobal flags:\n")
	g := &flagSet{}
	rt.bindGlobals(g)
	writeFlagHelp(out, g)
}

func writeFlagHelp(out io.Writer, fs *flagSet) {
	flags := append([]*flagDef(nil), fs.flags...)
	sort.Slice(flags, func(i, j int) bool { return flags[i].name < flags[j].name })
	for _, f := range flags {
		label := "--" + f.name
		if f.short != 0 {
			label = "-" + string(f.short) + ", " + label
		}
		switch f.kind {
		case flagString, flagStrings:
			label += " string"
		case flagInt:
			label += " int"
		}
		if f.usage != "" {
			fmt.Fprintf(out, "  %-24s %s\n", label, f.usage)
		} else {
			fmt.Fprintf(out, "  %s\n", label)
		}
	}
}

func (c *Command) checkArgs(args []string) error {
	n := len(args)
	if n < c.minArgs || (c.maxArgs >= 0 && n > c.maxArgs) {
		if c.Use != "" {
			return usagef("usage: %s", c.Use)
		}
		return usagef("%s: unexpected arguments", c.Name)
	}
	return nil
}
