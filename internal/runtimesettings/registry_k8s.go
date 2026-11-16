package runtimesettings

import "time"

// Kubernetes cluster connection settings. Env values only seed the defaults.
const (
	KeyK8sClustersEnabled     = "k8s.clusters_enabled"
	KeyK8sInClusterEnabled    = "k8s.in_cluster_enabled"
	KeyK8sAllowPrivateAPI     = "k8s.allow_private_api" // gitleaks:allow
	KeyK8sRequestTimeout      = "k8s.request_timeout"
	KeyK8sKubeconfigMaxBytes  = "k8s.kubeconfig_max_bytes"
	KeyK8sMaxBodyBytes        = "k8s.max_body_bytes" // gitleaks:allow
	KeyK8sLogTailLinesMax     = "k8s.log_tail_lines_max"
	KeyK8sStatusPollInterval  = "k8s.status_poll_interval"
	k8sKubeconfigMaxBytesCeil = 8 << 20
	k8sMaxBodyBytesCeil       = 256 << 20
	k8sLogTailLinesCeil       = 100000
)

func init() {
	cat := CategoryExperimental
	boolDef := func(key, env, title, desc string, risk Risk) {
		Register(Definition{
			Key: key, Category: cat, Kind: KindBool, Default: "false", Env: env, Apply: ApplyLive, Risk: risk,
			Title: title, Description: desc,
		})
	}
	boolDef(KeyK8sClustersEnabled, "APP_K8S_CLUSTERS_ENABLED", "Kubernetes clusters",
		"Lets root connect external Kubernetes clusters and deploy apps to them. Off by default.", RiskNormal)
	boolDef(KeyK8sInClusterEnabled, "APP_K8S_IN_CLUSTER_ENABLED", "Kubernetes in-cluster mode",
		"Allows connecting the cluster Levelrail itself runs in. Off by default.", RiskWeakensSecurity)
	boolDef(KeyK8sAllowPrivateAPI, "APP_K8S_ALLOW_PRIVATE_API", "Kubernetes private API servers",
		"Allows cluster API servers on private or loopback addresses (local Kind or k3d, private networks). Metadata and link-local addresses stay refused.", RiskWeakensSecurity)
	Register(Definition{
		Key: KeyK8sRequestTimeout, Category: cat, Kind: KindDuration, Default: "15s", Env: "APP_K8S_REQUEST_TIMEOUT", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Second, 5*time.Minute),
		Title:      "Kubernetes request timeout", Description: "How long one call to a cluster API server may take, from 1s to 5m.",
	})
	registerK8sInt(KeyK8sKubeconfigMaxBytes, "APP_K8S_KUBECONFIG_MAX_BYTES", "65536", "Kubeconfig size cap",
		"Largest kubeconfig document accepted on connect, in bytes.", 1024, k8sKubeconfigMaxBytesCeil)
	registerK8sInt(KeyK8sMaxBodyBytes, "APP_K8S_MAX_BODY_BYTES", "8388608", "Kubernetes response size cap",
		"Largest response body read from a cluster API server, in bytes.", 1024, k8sMaxBodyBytesCeil)
	registerK8sInt(KeyK8sLogTailLinesMax, "APP_K8S_LOG_TAIL_LINES_MAX", "500", "Kubernetes log tail cap",
		"Most log lines one pod log request may return.", 1, k8sLogTailLinesCeil)
	Register(Definition{
		Key: KeyK8sStatusPollInterval, Category: cat, Kind: KindDuration, Default: "10s", Env: "APP_K8S_STATUS_POLL_INTERVAL", Apply: ApplyLive,
		CheckValue: durationRangeCheck(time.Second, time.Hour),
		Title:      "Kubernetes status poll interval", Description: "How often deployment status is refreshed from a cluster, from 1s to 1h.",
	})
}

func registerK8sInt(key, env, def, title, desc string, lo, hi int) {
	registerLimit(Definition{
		Key: key, Category: CategoryExperimental, Default: def, Env: env, Apply: ApplyLive,
		Title: title, Description: desc,
	}, lo, hi)
}
