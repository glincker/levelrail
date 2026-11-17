package runtimesettings

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

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
	KeyK8sDefaultNamespace    = "k8s.default_namespace"
	KeyK8sCPURequest          = "k8s.cpu_request"
	KeyK8sMemoryRequest       = "k8s.memory_request"
	KeyK8sCPULimit            = "k8s.cpu_limit" // gitleaks:allow
	KeyK8sMemoryLimit         = "k8s.memory_limit"
	KeyK8sMaxReplicas         = "k8s.max_replicas" // gitleaks:allow
	KeyK8sMaxEnvVars          = "k8s.max_env_vars"
	KeyK8sRunAsUser           = "k8s.run_as_user"
	KeyK8sAllowedNamespaces   = "k8s.allowed_namespaces"
	KeyK8sAllowUnverifiedHost = "k8s.allow_unverified_hosts"
	KeyK8sMaxEnvValueBytes    = "k8s.max_env_value_bytes"
	KeyK8sAllowClusterAdmin   = "k8s.allow_cluster_admin"
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
	registerK8sQuantity(KeyK8sCPURequest, "APP_K8S_CPU_REQUEST", "100m", "Kubernetes CPU request",
		"CPU request set on every deployed container, as a Kubernetes quantity such as 100m.")
	registerK8sQuantity(KeyK8sMemoryRequest, "APP_K8S_MEMORY_REQUEST", "128Mi", "Kubernetes memory request",
		"Memory request set on every deployed container, as a Kubernetes quantity such as 128Mi.")
	registerK8sQuantity(KeyK8sCPULimit, "APP_K8S_CPU_LIMIT", "500m", "Kubernetes CPU limit",
		"CPU limit set on every deployed container, as a Kubernetes quantity such as 500m.")
	registerK8sQuantity(KeyK8sMemoryLimit, "APP_K8S_MEMORY_LIMIT", "512Mi", "Kubernetes memory limit",
		"Memory limit set on every deployed container, as a Kubernetes quantity such as 512Mi.")
	Register(Definition{
		Key: KeyK8sDefaultNamespace, Category: cat, Kind: KindString, Default: "levelrail-managed", Env: "APP_K8S_DEFAULT_NAMESPACE", Apply: ApplyLive,
		CheckValue: func(v string) error {
			if !k8sDNSLabel.MatchString(v) {
				return fmt.Errorf("must be a lowercase DNS label")
			}
			return nil
		},
		Title: "Kubernetes default namespace", Description: "Namespace used when a deploy request names none. Created with the managed-by label when missing.",
	})
	registerK8sInt(KeyK8sMaxReplicas, "APP_K8S_MAX_REPLICAS", "20", "Kubernetes replica cap",
		"Most replicas one deploy request may ask for.", 1, 1000)
	registerK8sInt(KeyK8sMaxEnvVars, "APP_K8S_MAX_ENV_VARS", "64", "Kubernetes env variable cap",
		"Most environment variables one deploy request may carry.", 0, 1000)
	Register(Definition{
		Key: KeyK8sAllowedNamespaces, Category: cat, Kind: KindString, Default: "", Env: "APP_K8S_ALLOWED_NAMESPACES", Apply: ApplyLive,
		CheckValue: func(v string) error {
			for _, ns := range ParseK8sNamespaces(v) {
				if !k8sDNSLabel.MatchString(ns) {
					return fmt.Errorf("%q is not a lowercase DNS label", ns)
				}
			}
			return nil
		},
		Title:       "Kubernetes allowed existing namespaces",
		Description: "Comma separated namespaces a deploy may use although Levelrail did not create them. The default namespace is always allowed. Empty by default.",
	})
	boolDef(KeyK8sAllowUnverifiedHost, "APP_K8S_ALLOW_UNVERIFIED_HOSTS", "Kubernetes unverified ingress hosts",
		"Lets a deploy that sets skip_host_check proceed when the cluster refuses to list Ingresses, so host ownership cannot be verified.", RiskWeakensSecurity)
	boolDef(KeyK8sAllowClusterAdmin, "APP_K8S_ALLOW_CLUSTER_ADMIN", "Kubernetes cluster-admin credentials",
		"Lets a connect request that sets allow_cluster_admin store a credential with wildcard access to the whole cluster. Refused by default; create a namespaced Role with levelrail clusters rbac instead.", RiskWeakensSecurity)
	registerK8sInt(KeyK8sMaxEnvValueBytes, "APP_K8S_MAX_ENV_VALUE_BYTES", "4096", "Kubernetes env value size cap",
		"Longest value, in bytes, one deploy environment variable may carry.", 1, 1<<20)
	registerK8sInt(KeyK8sRunAsUser, "APP_K8S_RUN_AS_USER", "10001", "Kubernetes non-root user id",
		"User id forced on containers so they never run as root unless a deploy sets allow_root.", 1, 2147483647)
}

var (
	k8sDNSLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	k8sQuantity = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(m|k|M|G|T|Ki|Mi|Gi|Ti)?$`)
)

// ParseK8sNamespaces splits a comma list, dropping blanks.
func ParseK8sNamespaces(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ValidK8sQuantity reports whether v is a plain Kubernetes resource quantity.
func ValidK8sQuantity(v string) bool { return k8sQuantity.MatchString(v) }

func registerK8sQuantity(key, env, def, title, desc string) {
	Register(Definition{
		Key: key, Category: CategoryExperimental, Kind: KindString, Default: def, Env: env, Apply: ApplyLive,
		CheckValue: func(v string) error {
			if !ValidK8sQuantity(v) {
				return fmt.Errorf("must be a Kubernetes quantity such as 100m or 128Mi")
			}
			return nil
		},
		Title: title, Description: desc,
	})
}

func registerK8sInt(key, env, def, title, desc string, lo, hi int) {
	registerLimit(Definition{
		Key: key, Category: CategoryExperimental, Default: def, Env: env, Apply: ApplyLive,
		Title: title, Description: desc,
	}, lo, hi)
}
