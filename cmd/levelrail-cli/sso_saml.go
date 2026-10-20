package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// samlFlags are the editable connection fields; only flags given on the
// command line overwrite the stored value on update.
type samlFlags struct {
	entityID, ssoURL, certFile, metadataURL string
	emailAttr, nameAttr, groupsAttr         string
	domains, defaultRole                    string
	enabled, jit, syncRoles                 bool
}

func (f *samlFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.entityID, "idp-entity-id", "", "IDP entity ID (issuer)")
	fs.StringVar(&f.ssoURL, "idp-sso-url", "", "IDP single sign-on URL (https)")
	fs.StringVar(&f.certFile, "idp-cert-file", "", "file holding the one IDP signing certificate, PEM or base64")
	fs.StringVar(&f.metadataURL, "idp-metadata-url", "", "IDP metadata URL kept for reference (https)")
	fs.StringVar(&f.emailAttr, "email-attribute", "", "assertion attribute holding the email (default: common names)")
	fs.StringVar(&f.nameAttr, "name-attribute", "", "assertion attribute holding the display name")
	fs.StringVar(&f.groupsAttr, "groups-attribute", "", "assertion attribute holding group names")
	fs.StringVar(&f.domains, "allowed-domains", "", "comma-separated email domains allowed to sign in (empty: any)")
	fs.StringVar(&f.defaultRole, "default-role", "", "role id for users no group mapping matches (empty: refuse them)")
	fs.BoolVar(&f.enabled, "enabled", false, "show the connection on the sign-in page")
	fs.BoolVar(&f.jit, "jit", false, "create accounts on first sign-in")
	fs.BoolVar(&f.syncRoles, "sync-roles", false, "reapply the mapped role on every sign-in")
}

func (f *samlFlags) apply(fs *flag.FlagSet, req *apiclient.SAMLConnectionRequest) error {
	var err error
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "idp-entity-id":
			req.IDPEntityID = f.entityID
		case "idp-sso-url":
			req.IDPSSOURL = f.ssoURL
		case "idp-cert-file":
			raw, rerr := os.ReadFile(f.certFile) //nolint:gosec // the operator names the file on their own command line
			if rerr != nil {
				err = newValidationError("read --idp-cert-file: %v", rerr)
			}
			req.IDPCertificates = string(raw)
		case "idp-metadata-url":
			req.IDPMetadataURL = f.metadataURL
		case "email-attribute":
			req.EmailAttribute = f.emailAttr
		case "name-attribute":
			req.NameAttribute = f.nameAttr
		case "groups-attribute":
			req.GroupsAttribute = f.groupsAttr
		case "allowed-domains":
			req.AllowedDomains = splitAndTrim(f.domains)
		case "default-role":
			req.DefaultRoleID = f.defaultRole
		case "enabled":
			req.Enabled = f.enabled
		case "jit":
			req.JITProvisioning = f.jit
		case "sync-roles":
			req.SyncRoles = f.syncRoles
		}
	})
	return err
}

func (c cliCtx) printSAML(conns ...apiclient.SAMLConnection) func() {
	return func() {
		tw := tabwriter.NewWriter(c.stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tENABLED\tCONFIGURED\tJIT\tACS URL")
		for _, s := range conns {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%t\t%t\t%s\n", s.ID, s.Name, s.Enabled, s.Configured, s.JITProvisioning, s.SPACSURL)
		}
		_ = tw.Flush()
	}
}

func oneArg(fs *flag.FlagSet, what string) (string, error) {
	if fs.NArg() != 1 {
		return "", newValidationError("expected exactly one %s", what)
	}
	return fs.Arg(0), nil
}

func (c cliCtx) runSAML(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, ssoUsage(c.prog))
		return exitUsage
	}
	rest := args[1:]
	var f samlFlags
	switch args[0] {
	case "list":
		return c.runAPI("sso saml list", "sso saml list [flags]", rest, nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				conns, err := cl.ListSAMLConnections(ctx)
				return conns, c.printSAML(conns...), err
			})
	case "get":
		return c.runAPI("sso saml get", "sso saml get <id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "connection id")
				if err != nil {
					return nil, nil, err
				}
				s, err := cl.GetSAMLConnection(ctx, id)
				return s, c.printSAML(s), err
			})
	case "create", "add":
		return c.runAPI("sso saml add", "sso saml create <name> [connection flags] [flags]", rest, f.register,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				name, err := oneArg(fs, "connection name")
				if err != nil {
					return nil, nil, err
				}
				req := apiclient.SAMLConnectionRequest{Name: name}
				if err := f.apply(fs, &req); err != nil {
					return nil, nil, err
				}
				s, err := cl.CreateSAMLConnection(ctx, req)
				return s, c.printSAML(s), err
			})
	case "update":
		return c.runAPI("sso saml update", "sso saml update <id> [connection flags] [flags]", rest, f.register,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "connection id")
				if err != nil {
					return nil, nil, err
				}
				cur, err := cl.GetSAMLConnection(ctx, id)
				if err != nil {
					return nil, nil, err
				}
				req := apiclient.RequestFromSAMLConnection(cur)
				if err := f.apply(fs, &req); err != nil {
					return nil, nil, err
				}
				s, err := cl.UpdateSAMLConnection(ctx, id, req)
				return s, c.printSAML(s), err
			})
	case "delete", "remove":
		return c.runAPI("sso saml remove", "sso saml remove <id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "connection id")
				if err != nil {
					return nil, nil, err
				}
				if err := cl.DeleteSAMLConnection(ctx, id); err != nil {
					return nil, nil, err
				}
				_, _ = fmt.Fprintf(c.stdout, "saml connection %s deleted\n", id)
				return nil, nil, nil
			})
	case "test":
		return c.runAPI("sso saml test", "sso saml test <id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "connection id")
				if err != nil {
					return nil, nil, err
				}
				s, err := cl.GetSAMLConnection(ctx, id)
				if err != nil {
					return nil, nil, err
				}
				if !s.Configured || !s.Enabled {
					return nil, nil, newValidationError("connection %s must be configured and enabled before it can be tested", id)
				}
				return printTestLogin(c, cl.URL("/api/v1/sso/saml/connections/"+apiclient.PathEscape(id)+"/test/start"))
			})
	case "import-metadata":
		return c.runSAMLImport(rest)
	case "metadata":
		return c.runAPI("sso saml metadata", "sso saml metadata <id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "connection id")
				if err != nil {
					return nil, nil, err
				}
				md, err := cl.SAMLSPMetadata(ctx, id)
				if err != nil {
					return nil, nil, err
				}
				_, _ = c.stdout.Write(md)
				return nil, nil, nil
			})
	}
	_, _ = fmt.Fprintf(c.stderr, "%s: unknown sso saml subcommand %q\n", c.prog, args[0])
	return exitUsage
}

func (c cliCtx) runSAMLImport(args []string) int {
	var metaURL, file string
	return c.runAPI("sso saml import-metadata", "sso saml import-metadata <id> (--url URL | --file FILE) [flags]", args,
		func(fs *flag.FlagSet) {
			fs.StringVar(&metaURL, "url", "", "https URL of the IDP metadata")
			fs.StringVar(&file, "file", "", "file holding the IDP metadata XML")
		},
		func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
			id, err := oneArg(fs, "connection id")
			if err != nil {
				return nil, nil, err
			}
			if (metaURL == "") == (file == "") {
				return nil, nil, newValidationError("give exactly one of --url or --file")
			}
			req := apiclient.SAMLMetadataImport{URL: metaURL}
			if file != "" {
				raw, err := os.ReadFile(file) //nolint:gosec // the operator names the file on their own command line
				if err != nil {
					return nil, nil, newValidationError("read --file: %v", err)
				}
				req.XML = string(raw)
			}
			s, err := cl.ImportSAMLMetadata(ctx, id, req)
			return s, c.printSAML(s), err
		})
}

// testLoginResult is what the test commands print: the sign-in runs in a
// browser, because the identity provider's login page needs one.
type testLoginResult struct {
	URL string `json:"url"`
}

func printTestLogin(c cliCtx, u string) (any, func(), error) {
	return testLoginResult{URL: u}, func() {
		_, _ = fmt.Fprintf(c.stdout, "Open this address in a browser signed in as a root user:\n  %s\nThe dashboard then shows the claims received, the mapped role and a typed fix for any failure. Nothing is created.\n", u)
	}, nil
}
