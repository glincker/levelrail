package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"golang.org/x/term"
)

func deleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s delete <kind> <name> [--yes] [flags]

Deletes an app or a database. Asks for confirmation on a terminal; pass
--yes in scripts. Data volumes are kept: use "%[1]s apps delete --delete-data"
when the data must go too. Nodes are removed with "%[1]s nodes delete".

Flags:
  --yes           do not ask for confirmation
  -n, --namespace string   require the resource to be in this project
  --json          print {"deleted": true} as JSON and nothing else
%[2]s`, prog, commonFlagsHelp)
}

func editUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s edit app <name> [flags]

Exports the app as a resource document, opens it in $VISUAL or $EDITOR, and
applies only the fields you changed with server-side apply. The save carries
the version you opened, so the server refuses it when the app changed while
you were editing and nothing is overwritten silently. Your edit is kept
in a file whose path is printed on conflict.

Flags:
  --yes           apply without asking after the editor closes
  --json          print the apply result as JSON and nothing else
%[2]s`, prog, commonFlagsHelp)
}

// confirmYes asks on a terminal; without one the caller must pass --yes.
func confirmYes(stderr io.Writer, question string) error {
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to continue without confirmation")
	}
	_, _ = fmt.Fprintf(stderr, "%s [y/N] ", question)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

func runDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var yes *bool
	var namespace *string
	c, code, ok := parseCLICall(prog, "delete", deleteUsage(prog), expandShortFlags(args), stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		yes = fs.Bool("yes", false, "do not ask for confirmation")
		namespace = fs.String("namespace", "", "require the resource to be in this project")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 2 {
		return c.invalid("delete needs a kind and a name, for example: delete app web")
	}
	kind, err := resolveResourceKind(rest[0])
	if err != nil {
		return c.fail(err)
	}
	if kind.Path == "nodes" {
		return c.invalid("nodes are not deleted from here: use \"%s nodes delete %s\"", prog, rest[1])
	}
	name := rest[1]
	if *namespace != "" {
		obj, gerr := c.client.GetV2(context.Background(), kind.Path, name)
		if gerr != nil {
			return c.fail(fmt.Errorf("delete %s %q: %w", kind.Singular, name, gerr))
		}
		if err := namespaceMismatch(*namespace, obj); err != nil {
			return c.fail(err)
		}
	}
	if !*yes {
		if err := confirmYes(stderr, fmt.Sprintf("Delete %s %q?", kind.Singular, name)); err != nil {
			return c.fail(err)
		}
	}
	ctx := context.Background()
	pending := false
	switch kind.Path {
	case "apps":
		res, derr := c.client.DeleteAppResult(ctx, name)
		err, pending = derr, res.Pending
	default:
		err = c.client.DeleteDatabase(ctx, name, false)
	}
	if err != nil {
		return c.fail(fmt.Errorf("delete %s %q: %w", kind.Singular, name, err))
	}
	payload := map[string]any{"deleted": true, "kind": kind.Singular, "name": name}
	if pending {
		payload["teardown_pending"] = true
	}
	return c.render(payload, func() { _, _ = fmt.Fprintf(c.stdout, "%s/%s deleted\n", kind.Singular, name) })
}

func runEdit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var yes *bool
	c, code, ok := parseCLICall(prog, "edit", editUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		yes = fs.Bool("yes", false, "apply without asking after the editor closes")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 2 {
		return c.invalid("edit needs a kind and a name, for example: edit app web")
	}
	kind, err := resolveResourceKind(rest[0])
	if err != nil {
		return c.fail(err)
	}
	if kind.Path != "apps" {
		return c.invalid("edit supports apps only: databases and nodes have no editable document yet")
	}
	name := rest[1]
	ctx := context.Background()

	before, err := c.client.GetV2(ctx, kind.Path, name)
	if err != nil {
		return c.fail(fmt.Errorf("edit app %q: %w", name, err))
	}
	exported, err := c.client.ExportIaC(ctx, "", name, true)
	if err != nil {
		return c.fail(fmt.Errorf("export app %q: %w", name, err))
	}
	original := exported.Join()

	dir, err := os.MkdirTemp("", "edit-")
	if err != nil {
		return c.fail(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		return c.fail(err)
	}
	if err := editorRunner(pickEditor(lookupEnv), path); err != nil {
		return c.fail(err)
	}
	edited, err := os.ReadFile(path) //nolint:gosec // the temp file this command just created
	if err != nil {
		return c.fail(err)
	}
	if string(edited) == original {
		_, _ = fmt.Fprintln(stderr, "no changes, nothing applied")
		return exitOK
	}

	origSpec, err := manifestSpec([]byte(original))
	if err != nil {
		return c.fail(fmt.Errorf("read exported app %q: %w", name, err))
	}
	editedSpec, err := manifestSpec(edited)
	if err != nil {
		return c.fail(fmt.Errorf("read your edit: %w", err))
	}
	changes := specChanges(origSpec, editedSpec)
	if len(changes) == 0 {
		_, _ = fmt.Fprintln(stderr, "no changes, nothing applied")
		return exitOK
	}
	if !*yes {
		if err := confirmYes(stderr, "Apply this change?"); err != nil {
			return c.fail(err)
		}
	}
	// The resourceVersion precondition makes the save a compare-and-swap on the server.
	manifest := map[string]any{"kind": "App", "metadata": map[string]any{"name": name}, "spec": changes}
	res, err := c.client.ApplyV2(ctx, kind.Path, name, manifest, apiclient.V2ApplyOptions{
		FieldManager: defaultFieldManager(prog) + "-edit", Force: true, ResourceVersion: before.Metadata.ResourceVersion,
	})
	if err != nil {
		keep := filepath.Join(os.TempDir(), "edit-"+filepath.Base(name)+".yaml")
		if werr := os.WriteFile(keep, edited, 0o600); werr != nil { //nolint:gosec // the name is a path.Base'd resource name under the temp dir
			keep = "(could not save)"
		}
		return c.fail(fmt.Errorf("apply app %q: %w; your edit is kept at %s", name, err, keep))
	}
	return c.render(res, func() { _, _ = fmt.Fprintf(stdout, "app/%s edited (version %s)\n", name, res.Metadata.ResourceVersion) })
}
