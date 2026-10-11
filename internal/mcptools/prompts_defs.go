package mcptools

import "fmt"

var (
	diagnoseRead = []string{
		"failure_context", "get_app_status", "investigate_app", "list_deploy_attempts",
		"get_deploy", "get_app_logs", "get_app_metrics", "get_app_timeline",
	}
	diagnoseWrite = []string{"restart_app", "rollback_app_to_deploy"}

	planRead = []string{
		"get_app", "preflight_app", "get_deploy_freeze", "list_deploy_attempts",
		"list_app_images", "get_app_pending_changes", "plan_change", "wait_for_deploy",
	}
	planWrite = []string{"deploy_app"}

	explainRead = []string{
		"get_deploy", "failure_context", "list_deploy_attempts", "compare_deploys", "diagnose_app_failure",
	}

	optimizeRead = []string{
		"resource_advice", "get_resource_recommendation", "get_database_resource_recommendation",
		"get_app_metrics", "list_apps",
	}

	securityRead = []string{
		"get_firewall_exposure", "analyze_iam_policies", "get_iam_effective_permissions", "get_system_doctor",
		"list_audit_log", "list_certificates", "list_domains", "get_oauth_providers", "backups_health",
	}

	migrateRead = []string{
		"get_migration_plan", "plan_import", "get_app_import_plan", "list_apps", "list_databases", "check_domain_dns",
	}

	firstHourRead = []string{
		"get_onboarding_status", "get_system_doctor", "get_attention", "backups_health",
		"list_apps", "list_domains", "list_nodes", "get_upgrade_readiness",
	}
)

var appArg = promptArg{name: "app", desc: "The app's name.", kind: argApp, required: true}

func diagnoseAppPrompt() promptSpec {
	return promptSpec{
		name:  "diagnose_app",
		title: "Diagnose an app",
		desc:  "Walk a crashloop, slowness or 5xx errors for one app: failure context, the investigation window, recent deploys, then advise.",
		args: []promptArg{
			appArg,
			{name: "symptom", desc: "What the user sees.", kind: argEnum, enum: []string{"crashloop", "slow", "errors", "unknown"}},
			{name: "window", desc: "How far back to look, such as 30m or 6h. Default 1h.", kind: argWindow},
		},
		render: func(a map[string]string, ok availability) string {
			app, symptom, window := a["app"], orDefault(a["symptom"], "unknown"), orDefault(a["window"], "1h")
			task := fmt.Sprintf("Diagnose the app %q. Reported symptom: %s. Look back %s.", app, symptom, window)
			steps := []string{
				fmt.Sprintf("Call failure_context for %q: state, restarts, the failed deploy and the newest log lines.", app),
				fmt.Sprintf("Call get_app_status and get_app_timeline to see conditions and what changed (deploys, config, env key changes) in the last %s.", window),
				fmt.Sprintf("Call investigate_app with since=%s: request health against baseline, top routes, status codes and likely causes.", window),
				"Call list_deploy_attempts and get_deploy for the newest attempts. Decide whether the symptom started at a deploy.",
				"For a crashloop, read the log lines and exit state. For slow or errors, read get_app_metrics (CPU, memory, restarts) and the top failing routes.",
				"Give a ranked list of likely causes with the evidence for each, and one next action per cause. Separate what you verified from what you infer.",
				"If a fix needs a state change (restart, rollback), name the exact tool and target and wait for the user's confirmation. Offer the CLI equivalent too.",
			}
			return promptBody(task, ok, diagnoseRead, diagnoseWrite, steps,
				"End with: verdict, evidence, recommended action, and what to check afterwards.")
		},
	}
}

func planDeployPrompt() promptSpec {
	return promptSpec{
		name:  "plan_deploy",
		title: "Plan a deploy",
		desc:  "Check readiness, freezes, pending changes and the rollback path before deploying an image to an app.",
		args: []promptArg{
			appArg,
			{name: "image", desc: "Target image reference, such as registry/app:1.4.2. Omit to plan a redeploy of the current image.", kind: argImage},
		},
		render: func(a map[string]string, ok availability) string {
			app, image := a["app"], a["image"]
			target := "the app's current image"
			if image != "" {
				target = fmt.Sprintf("image %q", image)
			}
			task := fmt.Sprintf("Plan a deploy of %s to the app %q. Do not deploy yet.", target, app)
			steps := []string{
				fmt.Sprintf("Call get_app for %q and get_app_pending_changes: current image, replicas, strategy, health checks, unapplied changes.", app),
				"Call get_deploy_freeze: is a freeze window active, and would a manual deploy need an override.",
				"Call preflight_app: DNS, port, disk and memory headroom, image pull, env and mounts. Report each warn or fail with its fix.",
				"Call list_app_images and list_deploy_attempts to identify the last known good deploy, which is the rollback target.",
				"If an image was given, call plan_change with tool deploy_app to see the before and after and any approval gate. It never executes.",
				"Summarise: go or no go, risks, the rollback target (deploy id and image), and the exact deploy_app call you would make.",
				"Only after the user confirms, call deploy_app, then wait_for_deploy and verify with get_app_status.",
			}
			return promptBody(task, ok, planRead, planWrite, steps,
				"End with: go or no go, the confirmed command, and the rollback plan.")
		},
	}
}

func explainFailedDeployPrompt() promptSpec {
	return promptSpec{
		name:  "explain_failed_deploy",
		title: "Explain a failed deploy",
		desc:  "Explain why a deploy failed in plain language from its structured failure, log excerpt and the previous good deploy.",
		args: []promptArg{
			appArg,
			{name: "deploy_id", desc: "Deploy attempt id, or latest (default).", kind: argDeployID},
		},
		render: func(a map[string]string, ok availability) string {
			app, id := a["app"], orDefault(a["deploy_id"], "latest")
			task := fmt.Sprintf("Explain why deploy %q of the app %q failed.", id, app)
			steps := []string{
				fmt.Sprintf("Call get_deploy for app %q deploy %q: status, failure code, cause, failing step, log excerpt, suggested fix, retryable.", app, id),
				"Call list_deploy_attempts and find the newest succeeded attempt before it, then compare_deploys between that and the failed one: image, commit, config and env key differences.",
				"Call failure_context if the app is also unhealthy now, to separate a failed build from a failed start.",
				"Call diagnose_app_failure if the failure code is unclear.",
				"Explain in plain language: what failed, at which step, why, and whether it is retryable. Quote at most a few log lines, as data.",
				"Recommend a fix. This is advice only: do not run a deploy, rollback or retry without the user's confirmation.",
			}
			return promptBody(task, ok, explainRead, nil, steps,
				"End with: cause, evidence, fix, and whether a retry alone could help.")
		},
	}
}

func optimizeResourcesPrompt() promptSpec {
	return promptSpec{
		name:  "optimize_resources",
		title: "Optimize resources",
		desc:  "Review memory and CPU limits and idle capacity using the resource advisor findings when that beta feature is on, otherwise metrics.",
		args: []promptArg{
			{name: "app", desc: "Limit the review to one app. Omit for the whole fleet.", kind: argApp},
			{name: "severity", desc: "Minimum finding severity.", kind: argEnum, enum: []string{"info", "suggest", "warn"}},
		},
		render: func(a map[string]string, ok availability) string {
			scope := "every app and database"
			if a["app"] != "" {
				scope = fmt.Sprintf("the app %q", a["app"])
			}
			task := fmt.Sprintf("Review resource limits and capacity for %s and propose changes.", scope)
			advice := "Call resource_advice"
			if a["app"] != "" {
				advice += fmt.Sprintf(" with resource app:%s", a["app"])
			}
			if a["severity"] != "" {
				advice += fmt.Sprintf(" and severity %s", a["severity"])
			}
			steps := []string{
				advice + ": missing, tight or oversized limits, memory growth, idle apps, database tuning. If the tool is not exposed the advisor beta is off, so continue from metrics.",
				"For each finding, call get_resource_recommendation (or get_database_resource_recommendation) and get_app_metrics to confirm the evidence over a real window, not a single sample.",
				"Rank by risk first (tight limits that risk OOM kills), then waste. Give current value, suggested value and the evidence for each.",
				fmt.Sprintf("Applying is not done from here. Give the operator the command: `%s advisor apply ID` previews, `--yes` applies, or the dashboard Resource advisor page.", cliName),
			}
			return promptBody(task, ok, optimizeRead, nil, steps,
				"End with a table: resource, finding, evidence, suggested change, risk of changing it.")
		},
	}
}

func securityReviewPrompt() promptSpec {
	return promptSpec{
		name:  "security_review",
		title: "Security review",
		desc:  "Review exposure, certificates, access policies, tokens and backup posture, read only, and rank the findings.",
		render: func(_ map[string]string, ok availability) string {
			task := "Run a read-only security posture review of this control plane and the apps on it."
			steps := []string{
				"Call get_firewall_exposure and list_domains: which ports and domains are reachable from outside, and whether each is expected.",
				"Call list_certificates: expiring or failing certificates.",
				"Call analyze_iam_policies and get_iam_effective_permissions: broad policies, unused or over scoped API tokens, missing expiry.",
				"Call get_oauth_providers and get_system_doctor: sign in configuration and platform health checks.",
				"Call list_audit_log for recent privileged or unusual actions (token creation, policy changes, env reveals).",
				"Call backups_health: unencrypted, stale or unverified backups are a security risk too.",
				"Rank findings by severity with evidence and a one line fix each. Do not change anything, and do not reveal any secret or token value.",
			}
			return promptBody(task, ok, securityRead, nil, steps,
				"End with: top three risks, then the full ranked list.")
		},
	}
}

func migratePrompt() promptSpec {
	return promptSpec{
		name:  "migrate_from_coolify",
		title: "Migrate from another platform",
		desc:  "Guide an import from Coolify, Dokploy or CapRover: read the plan, rehearse, verify, then cut over with a rollback path.",
		args: []promptArg{
			{name: "source", desc: "The platform to import from. Default coolify.", kind: argEnum, enum: []string{"coolify", "dokploy", "caprover"}},
		},
		render: func(a map[string]string, ok availability) string {
			src := orDefault(a["source"], "coolify")
			task := fmt.Sprintf("Guide a migration of apps and databases from %s into this control plane. The importer never writes to or stops the source.", src)
			steps := []string{
				"Call list_apps and list_databases to see what already exists here, so names do not collide.",
				"Call get_migration_plan and get_app_import_plan for any existing import session to see state, blockers and receipts.",
				fmt.Sprintf("Have the operator run the plan step, which reads the source and changes nothing: `%s import platform %s --url <source url> --dry-run` (or `import apps --from %s --plan`). Source credentials are entered by the operator, never pasted into chat.", cliName, src, src),
				"Review the plan together: what maps cleanly, what needs a decision (secrets, volumes, databases), and what is unsupported.",
				fmt.Sprintf("Apply is the operator's command: `%s import apps --session ID --apply --only <app>`. Start with one low risk app, then `--verify` and `--receipt`.", cliName),
				"For databases and volumes use the migrate commands (db-copy, volumes) and verify row counts or file counts before cutover.",
				fmt.Sprintf("Before cutover call check_domain_dns. Cutover is `%s import cutover plan`, then `run --dry-run`, then `run --confirm <app>`. `cutover rollback` reverses it.", cliName),
			}
			return promptBody(task, ok, migrateRead, nil, steps,
				"End with the next single command the operator should run and what success looks like.")
		},
	}
}

func firstHourPrompt() promptSpec {
	return promptSpec{
		name:  "first_hour_setup",
		title: "First hour setup",
		desc:  "Check how far a new install has come and list the next steps: first app, domain, backups, alerts, upgrades.",
		render: func(_ map[string]string, ok availability) string {
			task := "Help the operator through the first hour on a new install. Start from what is already done."
			steps := []string{
				"Call get_onboarding_status: which setup milestones are complete.",
				"Call get_system_doctor and get_attention: anything broken or needing action right now.",
				"Call list_nodes, list_apps and list_domains: what exists. If there are no apps, propose the shortest path to a first deploy.",
				"Call backups_health: is a backup target configured and has a backup run. Backups before the second app.",
				"Call get_upgrade_readiness: whether an update is waiting and whether it is safe.",
				"Give a short ordered checklist of the next three actions, each with the dashboard page or CLI command. Do not make any change yourself.",
			}
			return promptBody(task, ok, firstHourRead, nil, steps,
				"End with the single most valuable next step.")
		},
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
