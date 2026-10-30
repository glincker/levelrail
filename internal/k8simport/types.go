package k8simport

import "gopkg.in/yaml.v3"

type intOrStr struct {
	Int int
	Str string
}

func (v *intOrStr) UnmarshalYAML(n *yaml.Node) error {
	if n.Tag == "!!int" {
		return n.Decode(&v.Int)
	}
	return n.Decode(&v.Str)
}

type labelSelector struct {
	MatchLabels map[string]string `yaml:"matchLabels"`
}

type workloadSpec struct {
	Replicas             *int                  `yaml:"replicas"`
	Selector             labelSelector         `yaml:"selector"`
	Template             podTemplate           `yaml:"template"`
	Strategy             struct{ Type string } `yaml:"strategy"`
	VolumeClaimTemplates []object              `yaml:"volumeClaimTemplates"`
	// Job and CronJob fields.
	Schedule          string                 `yaml:"schedule"`
	TimeZone          string                 `yaml:"timeZone"`
	Suspend           bool                   `yaml:"suspend"`
	ConcurrencyPolicy string                 `yaml:"concurrencyPolicy"`
	JobTemplate       struct{ Spec jobSpec } `yaml:"jobTemplate"`
	Parallelism       *int                   `yaml:"parallelism"`
	Completions       *int                   `yaml:"completions"`
	ActiveDeadline    *int                   `yaml:"activeDeadlineSeconds"`
}

type jobSpec struct {
	Template       podTemplate `yaml:"template"`
	Parallelism    *int        `yaml:"parallelism"`
	Completions    *int        `yaml:"completions"`
	ActiveDeadline *int        `yaml:"activeDeadlineSeconds"`
}

type podTemplate struct {
	Metadata struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec yaml.Node `yaml:"spec"`
}

type podSpec struct {
	Containers     []container `yaml:"containers"`
	InitContainers []container `yaml:"initContainers"`
	Volumes        []podVolume `yaml:"volumes"`
	RestartPolicy  string      `yaml:"restartPolicy"`
	HostNetwork    bool        `yaml:"hostNetwork"`
	HostPID        bool        `yaml:"hostPID"`
	HostIPC        bool        `yaml:"hostIPC"`
}

type container struct {
	Name         string        `yaml:"name"`
	Image        string        `yaml:"image"`
	Command      []string      `yaml:"command"`
	Args         []string      `yaml:"args"`
	Env          []envEntry    `yaml:"env"`
	EnvFrom      []envFrom     `yaml:"envFrom"`
	Ports        []cPort       `yaml:"ports"`
	Resources    resourceBlock `yaml:"resources"`
	Readiness    *probeIn      `yaml:"readinessProbe"`
	Liveness     *probeIn      `yaml:"livenessProbe"`
	Startup      *probeIn      `yaml:"startupProbe"`
	VolumeMounts []volumeMount `yaml:"volumeMounts"`
	Security     struct {
		Privileged   *bool `yaml:"privileged"`
		Capabilities struct {
			Add []string `yaml:"add"`
		} `yaml:"capabilities"`
	} `yaml:"securityContext"`
	Raw yaml.Node `yaml:"-"`
}

type envEntry struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value"`
	ValueFrom *struct {
		ConfigMapKeyRef  *keyRef   `yaml:"configMapKeyRef"`
		SecretKeyRef     *keyRef   `yaml:"secretKeyRef"`
		FieldRef         yaml.Node `yaml:"fieldRef"`
		ResourceFieldRef yaml.Node `yaml:"resourceFieldRef"`
	} `yaml:"valueFrom"`
}

type keyRef struct {
	Name     string `yaml:"name"`
	Key      string `yaml:"key"`
	Optional bool   `yaml:"optional"`
}

type envFrom struct {
	Prefix       string `yaml:"prefix"`
	ConfigMapRef *struct {
		Name string `yaml:"name"`
	} `yaml:"configMapRef"`
	SecretRef *struct {
		Name string `yaml:"name"`
	} `yaml:"secretRef"`
}

type cPort struct {
	Name          string `yaml:"name"`
	ContainerPort int    `yaml:"containerPort"`
	HostPort      int    `yaml:"hostPort"`
	Protocol      string `yaml:"protocol"`
}

type resourceBlock struct {
	Requests map[string]string `yaml:"requests"`
	Limits   map[string]string `yaml:"limits"`
}

type probeIn struct {
	HTTPGet *struct {
		Path   string   `yaml:"path"`
		Port   intOrStr `yaml:"port"`
		Scheme string   `yaml:"scheme"`
	} `yaml:"httpGet"`
	TCPSocket *struct {
		Port intOrStr `yaml:"port"`
	} `yaml:"tcpSocket"`
	Exec *struct {
		Command []string `yaml:"command"`
	} `yaml:"exec"`
	GRPC             yaml.Node `yaml:"grpc"`
	PeriodSeconds    int       `yaml:"periodSeconds"`
	TimeoutSeconds   int       `yaml:"timeoutSeconds"`
	FailureThreshold int       `yaml:"failureThreshold"`
	InitialDelay     int       `yaml:"initialDelaySeconds"`
}

type volumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	ReadOnly  bool   `yaml:"readOnly"`
	SubPath   string `yaml:"subPath"`
}

type podVolume struct {
	Name string `yaml:"name"`
	PVC  *struct {
		ClaimName string `yaml:"claimName"`
	} `yaml:"persistentVolumeClaim"`
	EmptyDir  *yaml.Node `yaml:"emptyDir"`
	HostPath  *yaml.Node `yaml:"hostPath"`
	ConfigMap *struct {
		Name string `yaml:"name"`
	} `yaml:"configMap"`
	Secret *struct {
		SecretName string `yaml:"secretName"`
	} `yaml:"secret"`
}

type servicePort struct {
	Name       string   `yaml:"name"`
	Port       int      `yaml:"port"`
	TargetPort intOrStr `yaml:"targetPort"`
	NodePort   int      `yaml:"nodePort"`
	Protocol   string   `yaml:"protocol"`
}

type serviceSpec struct {
	Type      string            `yaml:"type"`
	Selector  map[string]string `yaml:"selector"`
	Ports     []servicePort     `yaml:"ports"`
	ClusterIP string            `yaml:"clusterIP"`
}

type ingressSpec struct {
	Rules []struct {
		Host string `yaml:"host"`
		HTTP struct {
			Paths []struct {
				Path    string `yaml:"path"`
				Backend struct {
					Service struct {
						Name string `yaml:"name"`
					} `yaml:"service"`
				} `yaml:"backend"`
			} `yaml:"paths"`
		} `yaml:"http"`
	} `yaml:"rules"`
	TLS yaml.Node `yaml:"tls"`
}
