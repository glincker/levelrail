package api

import "github.com/GLINCKER/levelrail/internal/spec"

// Suggestion identifiers are stable: the dashboard keys its i18n copy and
// per-user dismissal on them.
const (
	suggestWebhookMissing   = "webhook_not_received"
	suggestWebhookSignature = "webhook_signature_failing"
	suggestReleaseOnly      = "release_trigger_only"
	suggestBuildSettings    = "fix_build_settings"
	suggestPreviews         = "enable_previews"
	suggestHealthCheck      = "add_health_check"
	suggestNotifications    = "add_deploy_notifications"
	suggestHeadFailed       = "head_failed_to_build"
)

// Suggestion actions the dashboard maps to a click target.
const (
	suggestActionOpenSource        = "open_source_settings"
	suggestActionOpenBuildSettings = "open_build_settings"
	suggestActionEnablePreviews    = "enable_previews"
	suggestActionOpenHealth        = "open_health_settings"
	suggestActionOpenNotifications = "open_notifications"
	suggestActionOpenDeployLogs    = "open_deploy_logs"
)

const (
	severityWarning = "warning"
	severityInfo    = "info"
)

type deploySuggestionAction struct {
	Type     string `json:"type"`
	DeployID string `json:"deploy_id,omitempty"`
}

// deploySuggestion is one opinionated next step. Reason is English for CLI
// and API consumers; the dashboard translates by ID and Params.
type deploySuggestion struct {
	ID       string                 `json:"id"`
	Severity string                 `json:"severity"`
	Reason   string                 `json:"reason"`
	Params   map[string]string      `json:"params,omitempty"`
	Action   deploySuggestionAction `json:"action"`
}

// suggestionInput is every fact the rules read, gathered once by the handler
// so the rules stay a pure function.
type suggestionInput struct {
	Provider            string
	TriggerMode         string
	WebhookSeen         bool
	WebhookBadSignature bool
	NeedsBuildSettings  bool
	LastFailureCode     string
	PreviewEnabled      bool
	HasHealthCheck      bool
	HasNotifyTarget     bool
	ChannelsExist       bool
	HeadSHA             string
	HeadFailedDeployID  string
	BaseDirectory       string
	LooksLikeMonorepo   bool
}

func previewCapableProvider(provider string) bool {
	switch provider {
	case "github", "gitlab", "gitea":
		return true
	}
	return false
}

func buildConfigFailureCode(code string) bool {
	return code == "build_config_error" || code == "dockerfile_error"
}

// computeDeploySuggestions returns the suggestions that apply, warnings
// first, in a stable order.
func computeDeploySuggestions(in suggestionInput) []deploySuggestion {
	var out []deploySuggestion

	if in.HeadFailedDeployID != "" {
		out = append(out, deploySuggestion{
			ID: suggestHeadFailed, Severity: severityWarning,
			Reason: "The newest commit on this branch failed to build. Check the build log before deploying again.",
			Params: map[string]string{"sha": shortSHA(in.HeadSHA)},
			Action: deploySuggestionAction{Type: suggestActionOpenDeployLogs, DeployID: in.HeadFailedDeployID},
		})
	}
	if in.WebhookBadSignature {
		out = append(out, deploySuggestion{
			ID: suggestWebhookSignature, Severity: severityWarning,
			Reason: "The last webhook delivery was rejected for an invalid signature, so pushes are not deploying. Rotate the webhook secret or re-add the webhook.",
			Action: deploySuggestionAction{Type: suggestActionOpenSource},
		})
	}
	if in.NeedsBuildSettings || buildConfigFailureCode(in.LastFailureCode) || (in.LooksLikeMonorepo && in.BaseDirectory == "") {
		out = append(out, deploySuggestion{
			ID: suggestBuildSettings, Severity: severityWarning,
			Reason: "This looks like a monorepo or the last build failed on build configuration. Pick the build directory and Dockerfile.",
			Action: deploySuggestionAction{Type: suggestActionOpenBuildSettings},
		})
	}
	if !in.WebhookSeen && !in.WebhookBadSignature && in.TriggerMode != spec.TriggerModeRelease {
		out = append(out, deploySuggestion{
			ID: suggestWebhookMissing, Severity: severityInfo,
			Reason: "No webhook delivery has reached this app yet, so pushes will not auto-deploy. Add the webhook to turn on deploy on push.",
			Action: deploySuggestionAction{Type: suggestActionOpenSource},
		})
	}
	if in.TriggerMode == spec.TriggerModeRelease {
		out = append(out, deploySuggestion{
			ID: suggestReleaseOnly, Severity: severityInfo,
			Reason: "This app deploys only on releases, so new commits will not deploy by themselves.",
			Action: deploySuggestionAction{Type: suggestActionOpenSource},
		})
	}
	if !in.PreviewEnabled && in.WebhookSeen && previewCapableProvider(in.Provider) {
		out = append(out, deploySuggestion{
			ID: suggestPreviews, Severity: severityInfo,
			Reason: "Pull request events are arriving. Enable preview environments to get a live URL per pull request.",
			Action: deploySuggestionAction{Type: suggestActionEnablePreviews},
		})
	}
	if !in.HasHealthCheck {
		out = append(out, deploySuggestion{
			ID: suggestHealthCheck, Severity: severityInfo,
			Reason: "No health check is configured, so a deploy that starts but does not serve still counts as healthy.",
			Action: deploySuggestionAction{Type: suggestActionOpenHealth},
		})
	}
	if !in.HasNotifyTarget {
		params := map[string]string{"channels_exist": "false"}
		if in.ChannelsExist {
			params["channels_exist"] = "true"
		}
		out = append(out, deploySuggestion{
			ID: suggestNotifications, Severity: severityInfo,
			Reason: "Nobody is notified when this app deploys or fails. Add a deploy notification channel.",
			Params: params,
			Action: deploySuggestionAction{Type: suggestActionOpenNotifications},
		})
	}
	return out
}
