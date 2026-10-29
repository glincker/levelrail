package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func labelUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s label <kind> <name> [key=value ...] [key- ...] [flags]

Shows or changes free-form labels on an app, database, stack or node.
key=value sets a label, key- removes one. Without pairs it prints the labels.
List by label with: %[1]s get apps -l env=prod,tier!=db

Flags:
  --json   print the labels as JSON and nothing else
%[2]s`, prog, commonFlagsHelp)
}

// labelPath maps a kind word to its label route segment; stacks are not in the v2 kinds.
func labelPath(word string) (path, singular string, err error) {
	if strings.EqualFold(word, "stack") || strings.EqualFold(word, "stacks") {
		return "stacks", "stack", nil
	}
	k, err := resolveResourceKind(word)
	if err != nil {
		return "", "", err
	}
	return k.Path, k.Singular, nil
}

// parseLabelArgs splits key=value and key- operands into a set map and a remove list.
func parseLabelArgs(pairs []string) (set map[string]string, remove []string, err error) {
	set = map[string]string{}
	for _, p := range pairs {
		switch {
		case strings.HasSuffix(p, "-") && !strings.Contains(p, "="):
			remove = append(remove, strings.TrimSuffix(p, "-"))
		case strings.Contains(p, "="):
			k, v, _ := strings.Cut(p, "=")
			if k == "" {
				return nil, nil, newValidationError("label %q needs a key before '='", p)
			}
			set[k] = v
		default:
			return nil, nil, newValidationError("label operand %q must be key=value or key-", p)
		}
	}
	return set, remove, nil
}

func runLabel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "label", labelUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 2 {
		return c.invalid("label needs a kind and a name, for example: label app web env=prod")
	}
	path, singular, err := labelPath(rest[0])
	if err != nil {
		return c.fail(err)
	}
	set, remove, err := parseLabelArgs(rest[2:])
	if err != nil {
		return c.fail(err)
	}
	ctx := context.Background()
	var res apiclient.LabelSet
	if len(rest) == 2 {
		res, err = c.client.GetResourceLabels(ctx, path, rest[1])
	} else {
		res, err = c.client.PatchResourceLabels(ctx, path, rest[1], set, remove)
	}
	if err != nil {
		return c.fail(fmt.Errorf("label %s %q: %w", singular, rest[1], err))
	}
	return c.render(res, func() {
		if len(res.Labels) == 0 {
			_, _ = fmt.Fprintf(c.stdout, "%s/%s has no labels\n", singular, rest[1])
			return
		}
		keys := make([]string, 0, len(res.Labels))
		for k := range res.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(c.stdout, "%s=%s\n", k, res.Labels[k])
		}
	})
}

// namespaceMismatch reports an object outside the namespace the caller asked for.
func namespaceMismatch(namespace string, o apiclient.V2Object) error {
	if namespace == "" || o.Metadata.Namespace == namespace {
		return nil
	}
	return newValidationError("%s %q is in namespace %q, not %q", strings.ToLower(o.Kind), o.Metadata.Name, o.Metadata.Namespace, namespace)
}
