package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// nsParams are the parameter names that carry a secrets namespace on a Manager method.
var nsParams = map[string]bool{"serviceName": true, "serviceNames": true, "from": true, "to": true, "app": true, "namespace": true}

// guardFuncs are the calls that refuse a reserved name for the expression passed to them.
var guardFuncs = map[string]bool{"AppNamespace": true, "IsReservedName": true, "ReservedNameCovers": true, "refuseReservedApp": true, "loadRevealApp": true}

type nsMethod struct {
	arity int
	pos   []int
}

// managerNamespaceMethods derives, from internal/secrets, every exported Manager method that takes a namespace.
func managerNamespaceMethods(t *testing.T) map[string]nsMethod {
	t.Helper()
	fset := token.NewFileSet()
	paths, err := filepath.Glob("../secrets/*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]nsMethod{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		{
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Recv == nil || !fd.Name.IsExported() || len(fd.Recv.List) != 1 {
					continue
				}
				if types.ExprString(fd.Recv.List[0].Type) != "*Manager" {
					continue
				}
				m := nsMethod{}
				i := 0
				for _, p := range fd.Type.Params.List {
					for _, n := range p.Names {
						if nsParams[n.Name] {
							m.pos = append(m.pos, i)
						}
						i++
					}
				}
				m.arity = i
				if len(m.pos) > 0 {
					out[fd.Name.Name] = m
				}
			}
		}
	}
	return out
}

// namespaceAllowlist names the call sites whose namespace is not routed through the reserved name guard, each with why it is safe.
// Key is "path under internal:function".
var namespaceAllowlist = map[string]string{ //nolint:gosec // call site names, not credentials
	"api/app_health_score.go:healthScoreSecrets":                         "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/app_import_run.go:rollbackAppImportItem":                        "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_compose.go:flush":                                          "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_compose.go:generateComposeSecret":                          "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_compose.go:pruneStaleComposeSecretFiles":                   "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_compose.go:storeComposeSecretFiles":                        "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_create.go:handleCreateApp":                                 "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_delete.go:parkAppSecrets":                                  "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/apps_delete.go:removeAppCredentials":                            "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/name_admission.go:dropUnsavedSecrets":                           "deletes only values this request wrote for names admitNewServiceName admitted, and only while the name has no service row",
	"api/apps_multi.go:handleDeploySpec":                                 "every derived service name and single_service_name passes admitNewServiceName under lockAppName before SetValueGuarded, and unsaved values are dropped",
	"api/cloud_do_oauth.go:loadDOOAuthBundle":                            "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/cloud_do_oauth.go:saveDOOAuthBundle":                            "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/database_cache_access.go:applyCacheAccessAtCreate":              "database namespace: database names are checked by the store name guard, not the app guard",
	"api/database_credentials_reveal.go:handleRevealDatabaseCredentials": "database namespace: database names are checked by the store name guard, not the app guard",
	"api/database_health_source.go:PoolerSaturation":                     "database namespace: database names are checked by the store name guard, not the app guard",
	"api/database_public_access.go:handleSetDatabasePublicAccess":        "database namespace: database names are checked by the store name guard, not the app guard",
	"api/database_tls_toggle.go:handleSetDatabaseTLSEnabled":             "database namespace: database names are checked by the store name guard, not the app guard",
	"api/databases.go:databaseTLSEnabled":                                "database namespace: database names are checked by the store name guard, not the app guard",
	"api/databases.go:databaseTLSStatuses":                               "database namespace: database names are checked by the store name guard, not the app guard",
	"api/domain_tls_cert.go:rollbackTLSCert":                             "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/domain_tls_cert.go:snapshotTLSCert":                             "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/email_providers.go:readSlots":                                   "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/email_providers.go:writeSlots":                                  "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/env_apply.go:buildEnvPlan":                                      "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/env_bound.go:boundDatabaseURL":                                  "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/env_bound.go:boundDatabaseValue":                                "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/env_reveal.go:resolveRevealValue":                               "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/env_revisions.go:diffEnvRevisions":                              "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/env_revisions.go:snapshotEnvRevision":                           "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/git_connection_health.go:readGitOAuthState":                     "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/k8s_clusters.go:handleConnectK8sCluster":                        "namespace is the cluster secrets key plus /pull, always slashed",
	"api/k8s_clusters.go:handleDisconnectK8sCluster":                     "namespace is the cluster secrets key plus /pull, always slashed",
	"api/k8s_pull_secrets.go:handleAddK8sPullSecret":                     "namespace is the cluster secrets key plus /pull, always slashed",
	"api/k8s_pull_secrets.go:handleDeleteK8sPullSecret":                  "namespace is the cluster secrets key plus /pull, always slashed",
	"api/k8s_pull_secrets.go:handleListK8sPullSecrets":                   "namespace is the cluster secrets key plus /pull, always slashed",
	"api/k8s_pull_secrets.go:loadK8sPullSecret":                          "namespace is the cluster secrets key plus /pull, always slashed",
	"api/mail_groups_api.go:audience":                                    "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/mail_mailboxes.go:AdminPassword":                                "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/migration_hub_receipt.go:buildHubConnection":                    "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"api/models_keys.go:handleListModelKeys":                             "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/platform_import.go:CreateApp":                                   "appNameFree (the new-name check behind admitNewServiceName) runs under lockAppName before SetValueGuarded, and unsaved values are dropped",
	"api/secret_rotate_value.go:handleRotateSecretValue":                 "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/secrets.go:applySecretBuildTime":                                "called from a handler that refuses a reserved app name first (refuseReservedApp or loadRevealApp)",
	"api/shared_env_secrets.go:handleSetSharedEnvSecret":                 "REVIEW: app or service scoped, creation names go through the store name guard; this site has no own guard",
	"cloudconn/oracle.go:OpenOracleSealed":                               "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"cloudconn/oracle.go:SealOracleSecret":                               "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"dbrotate/service.go:Preview":                                        "database namespace: database names are checked by the store name guard, not the app guard",
	"dbrotate/service.go:Rotate":                                         "database namespace: database names are checked by the store name guard, not the app guard",
	"dbrotate/service.go:commit":                                         "database namespace: database names are checked by the store name guard, not the app guard",
	"dbrotate/service.go:dropPending":                                    "database namespace: database names are checked by the store name guard, not the app guard",
	"dbrotate/service.go:settle":                                         "database namespace: database names are checked by the store name guard, not the app guard",
	"deploy/files.go:dropStaleFileSecrets":                               "only deletes slots of the previously saved record of the same service, so no new namespace is written",
	"deploy/files.go:dropUnsavedFileSecrets":                             "deletes only slots persistFileSecrets just wrote after the NameAdmitter (admitNewServiceName) passed, for a service with no saved record",
	"deploy/files.go:persistFileSecrets":                                 "the pipeline NameAdmitter (api admitNewServiceName) runs before the first file secret slot is written, and unsaved slots are dropped",
	"deployurls/guard.go:Exists":                                         "ParentSecrets.owner refuses a reserved or database-named parent and fails closed on store errors",
	"deployurls/guard.go:Resolve":                                        "ParentSecrets.owner refuses a reserved or database-named parent and fails closed on store errors",
	"models/usage.go:Usage":                                              "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"preview/manager.go:process":                                         "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"reconcile/application/controller.go:databaseTLSEnabled":             "database namespace: database names are checked by the store name guard, not the app guard",
	"reconcile/application/controller.go:resolveDatabaseField":           "database namespace: database names are checked by the store name guard, not the app guard",
	"reconcile/application/controller.go:resolveDatabasePassword":        "database namespace: database names are checked by the store name guard, not the app guard",
	"reconcile/application/controller.go:resolveIntegrationField":        "database namespace: database names are checked by the store name guard, not the app guard",
	"reconcile/application/controller.go:resolveRedisPassword":           "database namespace: database names are checked by the store name guard, not the app guard",
	"secretconn/service.go:checkRef":                                     "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"secretconn/service.go:rowCredential":                                "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"secrets/controlplane.go:MigrateKey":                                 "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"secrets/namespace_copy.go:CopyNamespace":                            "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
	"sharedenv/resolver.go:mergeSecrets":                                 "REVIEW: reconciler or internal service path, reserved names are neutralized upstream or the namespace is internal",
}

// builderNameRe matches the namespace builders and namespace constants that always yield a slashed or prefixed control plane key.
var builderNameRe = regexp.MustCompile(`(?i)(secretskey|secretsns|secretservice|secretns|secretref|secretprefix|secretsnamespace|credentialref|secretslot)$|^(keyservice|samlkeyservice|secretsservice|jobsecretsservice|tokenstateservice|oidcsigningkeyservicename|secretsservicename|secretsowner|ownerkey|osenv|servicename|oraclesealednamespace)$`)

func calleeName(e ast.Expr) string {
	switch fn := e.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// assignedExprs maps a local identifier to the single expression it is declared or assigned from.
func assignedExprs(body *ast.BlockStmt) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	multi := map[string]bool{}
	note := func(name string, v ast.Expr) {
		if _, dup := out[name]; dup {
			multi[name] = true
		}
		out[name] = v
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch st := n.(type) {
		case *ast.AssignStmt:
			if len(st.Lhs) == len(st.Rhs) {
				for i, l := range st.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						note(id.Name, st.Rhs[i])
					}
				}
			}
		case *ast.ValueSpec:
			for i, id := range st.Names {
				if i < len(st.Values) {
					note(id.Name, st.Values[i])
				}
			}
		}
		return true
	})
	for k := range multi {
		delete(out, k)
	}
	return out
}

// builderSafe reports whether the namespace expression is built by a control plane namespace builder, directly or through a local variable.
func builderSafe(e ast.Expr, assigns map[string]ast.Expr, depth int) bool {
	if depth > 4 {
		return false
	}
	switch x := e.(type) {
	case *ast.CallExpr:
		return builderNameRe.MatchString(calleeName(x.Fun))
	case *ast.SelectorExpr:
		return builderNameRe.MatchString(x.Sel.Name)
	case *ast.BinaryExpr:
		return x.Op == token.ADD && builderSafe(x.X, assigns, depth+1)
	case *ast.ParenExpr:
		return builderSafe(x.X, assigns, depth+1)
	case *ast.Ident:
		if builderNameRe.MatchString(x.Name) {
			return true
		}
		if v, ok := assigns[x.Name]; ok {
			return builderSafe(v, assigns, depth+1)
		}
	}
	return false
}

type nsSite struct {
	key, pos, method, expr string
}

func enclosingGuarded(body *ast.BlockStmt) map[string]bool {
	g := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) == 0 {
			return true
		}
		name := ""
		switch fn := c.Fun.(type) {
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		case *ast.Ident:
			name = fn.Name
		}
		if guardFuncs[name] {
			for _, a := range c.Args {
				g[types.ExprString(a)] = true
			}
		}
		return true
	})
	return g
}

// classifyManagerCalls fails on any secret Manager call whose namespace is neither a reserved literal, guarded, nor allowlisted.
func classifyManagerCalls(t *testing.T) {
	t.Helper()
	methods := managerNamespaceMethods(t)
	for _, want := range []string{"ListKeys", "DeleteAll", "CopyNamespace", "SealValue", "ExistsForServices", "SetBuildEnabled", "BuildEnabled", "SetValue", "Resolve"} {
		if _, ok := methods[want]; !ok {
			t.Fatalf("derived namespace methods miss %s, the derivation is broken: %v", want, methods)
		}
	}
	fset := token.NewFileSet()
	var bad []nsSite
	used := map[string]bool{}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		rel := strings.TrimPrefix(filepath.ToSlash(path), "../")
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			guarded := enclosingGuarded(fd.Body)
			assigns := assignedExprs(fd.Body)
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				m, ok := methods[sel.Sel.Name]
				if !ok || len(call.Args) != m.arity {
					return true
				}
				for _, p := range m.pos {
					arg := call.Args[p]
					if s, isLit := stringLit(arg); isLit {
						if !ReservedNameCovers(s) {
							bad = append(bad, nsSite{rel + ":" + fd.Name.Name, fset.Position(call.Pos()).String(), sel.Sel.Name, strconvQuote(s)})
						}
						continue
					}
					expr := types.ExprString(arg)
					if guarded[expr] || builderSafe(arg, assigns, 0) {
						continue
					}
					key := rel + ":" + fd.Name.Name
					if _, ok := namespaceAllowlist[key]; ok {
						used[key] = true
						continue
					}
					if _, ok := namespaceAllowlist[rel+":*"]; ok {
						used[rel+":*"] = true
						continue
					}
					bad = append(bad, nsSite{key, fset.Position(call.Pos()).String(), sel.Sel.Name, expr})
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(bad, func(i, j int) bool { return bad[i].pos < bad[j].pos })
	for _, b := range bad {
		t.Errorf("%s: %s namespace %s is not a reserved literal, guarded by secrets.AppNamespace in the same function, or listed in namespaceAllowlist as %q", b.pos, b.method, b.expr, b.key)
	}
	for k, why := range namespaceAllowlist {
		if why == "" {
			t.Errorf("allowlist entry %q has no reason", k)
		}
		if !used[k] {
			t.Errorf("allowlist entry %q matches no call site, remove it", k)
		}
	}
}

func strconvQuote(s string) string { return "\"" + s + "\"" }
