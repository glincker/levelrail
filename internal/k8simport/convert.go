// Package k8simport maps Kubernetes manifests onto Levelrail stack
// documents. It parses text only: it never fetches, executes or applies.
package k8simport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

var (
	envNameRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	nameCleanR = regexp.MustCompile(`[^a-z0-9-]+`)
)

type envVal struct {
	literal string
	secret  bool
}

type probeOut struct {
	path, scheme      string
	interval, timeout string
	failures          int
	exec              []string
}

type pubPort struct {
	container, host int
	protocol        string
}

type volOut struct{ name, path string }

type jobOut struct {
	name, schedule, tz, concurrency string
	command                         []string
	disabled                        bool
	timeout                         string
}

type app struct {
	name, ns, kind, srcName string
	image                   string
	port                    int
	replicas                int
	strategy, restart       string
	command                 []string
	env                     map[string]envVal
	memory                  string
	cpu                     float64
	readiness, liveness     *probeOut
	readyTimeout            string
	domains                 []string
	published               []pubPort
	volumes                 []volOut
	jobs                    []jobOut
	podLabels               map[string]string
	containerPorts          []cPort
	secretKeys              map[string]bool
	probePorts              []intOrStr
	portFromService         bool
}

type importer struct {
	opts      Options
	res       *Result
	configs   map[string]map[string]string
	secrets   map[string]map[string]string
	secretTyp map[string]string
	pvcs      map[string]object
	services  map[string]object
	apps      []*app
	usedCM    map[string]bool
	usedSec   map[string]bool
	usedPVC   map[string]bool
	names     map[string]string
}

// Convert maps Kubernetes manifests to stack documents plus a report.
func Convert(data []byte, opts Options) (*Result, error) {
	opts.Limits = opts.Limits.withDefaults()
	if opts.NamespaceAs == "" {
		opts.NamespaceAs = NamespaceEnvironment
	}
	if opts.NamespaceAs != NamespaceEnvironment && opts.NamespaceAs != NamespacePrefix && opts.NamespaceAs != NamespaceIgnore {
		return nil, fmt.Errorf("%w: namespace-as must be environment, prefix or ignore", ErrInput)
	}
	objs, bad, err := parseDocuments(data, opts.Limits)
	if err != nil {
		return nil, err
	}
	im := &importer{
		opts: opts, res: &Result{secrets: map[string]string{}},
		configs: map[string]map[string]string{}, secrets: map[string]map[string]string{}, secretTyp: map[string]string{},
		pvcs: map[string]object{}, services: map[string]object{},
		usedCM: map[string]bool{}, usedSec: map[string]bool{}, usedPVC: map[string]bool{}, names: map[string]string{},
	}
	for _, b := range bad {
		im.res.add(SeverityChangesBehavior, "Document", fmt.Sprintf("#%d", b.Index), "", b.Reason)
	}
	var workloads, services, ingresses []object
	for _, o := range objs {
		switch o.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
			workloads = append(workloads, o)
		case "Service":
			services = append(services, o)
			im.services[o.key()] = o
		case "Ingress":
			ingresses = append(ingresses, o)
		case "ConfigMap":
			im.configs[o.key()] = o.Data
		case "Secret":
			im.addSecret(o)
		case "PersistentVolumeClaim":
			im.pvcs[o.key()] = o
		case "Namespace", "ServiceAccount":
			im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, "", "not needed: namespaces follow --namespace-as and apps carry no service account")
		default:
			im.res.add(SeverityChangesBehavior, orDash(o.Kind), orDash(o.Metadata.Name), "", "kind is not supported by the importer and was skipped")
		}
	}
	for _, o := range workloads {
		im.workload(o)
	}
	for _, o := range services {
		im.service(o)
	}
	for _, o := range ingresses {
		im.ingress(o)
	}
	im.finish()
	return im.res, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (im *importer) addSecret(o object) {
	vals := map[string]string{}
	for k, v := range o.Data {
		raw, err := decodeBase64(v)
		if err != nil {
			im.res.add(SeverityChangesBehavior, "Secret", o.Metadata.Name, "data."+k, "value is not valid base64 and was skipped")
			continue
		}
		vals[k] = raw
	}
	for k, v := range o.StringData {
		vals[k] = v
	}
	im.secrets[o.key()] = vals
	im.secretTyp[o.key()] = o.Type
	switch o.Type {
	case "kubernetes.io/dockerconfigjson", "kubernetes.io/dockercfg":
		im.res.add(SeverityChangesBehavior, "Secret", o.Metadata.Name, "type", "registry credential is not imported: add it as a registry credential and name it on the app")
	case "kubernetes.io/tls":
		im.res.add(SeverityIgnoredSafe, "Secret", o.Metadata.Name, "type", "TLS key pair is not imported: certificates are issued automatically for domains")
	}
}

func (im *importer) appName(ns, name string) string {
	n := strings.ToLower(name)
	n = nameCleanR.ReplaceAllString(strings.ReplaceAll(n, ".", "-"), "-")
	n = strings.Trim(n, "-")
	if im.opts.NamespaceAs == NamespacePrefix && ns != "default" && ns != "" {
		n = ns + "-" + n
	}
	if im.opts.AppPrefix != "" {
		n = im.opts.AppPrefix + n
	}
	if n == "" || n[0] < 'a' || n[0] > 'z' {
		n = "app-" + n
	}
	if len(n) > 63 {
		n = strings.TrimRight(n[:63], "-")
	}
	return n
}

func (im *importer) workload(o object) {
	var ws workloadSpec
	if err := o.Spec.Decode(&ws); err != nil {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, "spec", "spec does not decode: "+trimDecodeErr(err))
		return
	}
	tpl := ws.Template
	if o.Kind == "CronJob" {
		tpl = ws.JobTemplate.Spec.Template
	}
	a := &app{name: im.appName(o.ns(), o.Metadata.Name), ns: o.ns(), kind: o.Kind, srcName: o.Metadata.Name, replicas: 1,
		env: map[string]envVal{}, podLabels: tpl.Metadata.Labels, secretKeys: map[string]bool{}}
	if prev, dup := im.names[a.name]; dup {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, "metadata.name", "app name "+a.name+" collides with "+prev+": use --app-prefix or --namespace-as")
		return
	}
	im.names[a.name] = o.Kind + "/" + o.Metadata.Name
	if ws.Replicas != nil {
		a.replicas = *ws.Replicas
	}
	if a.replicas < 1 {
		a.replicas = 1
		if ws.Replicas != nil {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.replicas", "zero replicas is not supported, imported with 1")
		}
	}
	im.checkKeys(o, "spec", &o.Spec, map[string]bool{"replicas": true, "template": true, "strategy": true, "volumeClaimTemplates": true,
		"schedule": true, "timeZone": true, "suspend": true, "concurrencyPolicy": true, "jobTemplate": true, "parallelism": true, "completions": true, "activeDeadlineSeconds": true, "selector": true, "revisionHistoryLimit": true, "progressDeadlineSeconds": true, "minReadySeconds": true,
		"serviceName": true, "podManagementPolicy": true, "updateStrategy": true, "persistentVolumeClaimRetentionPolicy": true,
		"backoffLimit": true, "ttlSecondsAfterFinished": true, "successfulJobsHistoryLimit": true, "failedJobsHistoryLimit": true,
		"startingDeadlineSeconds": true, "completionMode": true},
		nil)
	switch o.Kind {
	case "Deployment":
		switch ws.Strategy.Type {
		case "Recreate":
			a.strategy = "recreate"
		case "RollingUpdate":
			a.strategy = "rolling"
		}
	case "DaemonSet":
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "kind", "one pod per node is not supported, imported as a single replica")
	case "Job", "CronJob":
		inner := jobSpec{Parallelism: ws.Parallelism, Completions: ws.Completions}
		if o.Kind == "CronJob" {
			inner = ws.JobTemplate.Spec
		}
		if (inner.Parallelism != nil && *inner.Parallelism > 1) || (inner.Completions != nil && *inner.Completions > 1) {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.completions", "parallel or repeated completions run once")
		}
	}
	var ps podSpec
	if err := tpl.Spec.Decode(&ps); err != nil {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, "spec.template.spec", "pod spec does not decode: "+trimDecodeErr(err))
		return
	}
	im.checkKeys(o, "spec.template.spec", &tpl.Spec, podKnown, podSafeKeys)
	im.hardening(o, ps)
	if len(ps.Containers) == 0 {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, "spec.template.spec.containers", "no containers")
		return
	}
	main := 0
	for i, c := range ps.Containers {
		if c.Name == o.Metadata.Name {
			main = i
		}
	}
	for i, c := range ps.Containers {
		if i != main {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, fmt.Sprintf("spec.template.spec.containers[%d]", i),
				fmt.Sprintf("sidecar %q is not imported: containers of a pod share localhost, which services here do not; run it as its own app", c.Name))
		}
	}
	for i, c := range ps.InitContainers {
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, fmt.Sprintf("spec.template.spec.initContainers[%d]", i),
			fmt.Sprintf("init container %q does not run: move its work into the image or a pre-deploy hook", c.Name))
	}
	im.container(o, a, ps, ps.Containers[main], ws)
	switch o.Kind {
	case "Job", "CronJob":
		a.restart = "no"
		if ps.RestartPolicy == "OnFailure" {
			a.restart = "on-failure"
		}
	}
	if o.Kind == "CronJob" {
		im.cronJob(o, a, ws, ps.Containers[main])
	}
	im.apps = append(im.apps, a)
}

var podKnown = map[string]bool{"containers": true, "initContainers": true, "volumes": true, "restartPolicy": true, "hostNetwork": true, "hostPID": true, "hostIPC": true}

var podSafeKeys = map[string]bool{"serviceAccountName": true, "serviceAccount": true, "tolerations": true, "terminationGracePeriodSeconds": true,
	"topologySpreadConstraints": true, "priorityClassName": true, "priority": true, "hostname": true, "subdomain": true, "dnsPolicy": true,
	"automountServiceAccountToken": true, "schedulerName": true, "enableServiceLinks": true, "activeDeadlineSeconds": true, "preemptionPolicy": true}

func (im *importer) checkKeys(o object, path string, n *yaml.Node, known, safe map[string]bool) {
	for _, k := range mapKeys(n) {
		if known[k] {
			continue
		}
		sev := SeverityChangesBehavior
		reason := "field is not supported and was ignored"
		if safe[k] {
			sev, reason = SeverityIgnoredSafe, "no equivalent, and ignoring it does not change what runs"
		}
		im.res.add(sev, o.Kind, o.Metadata.Name, path+"."+k, reason)
	}
}

func (im *importer) hardening(o object, ps podSpec) {
	const p = "spec.template.spec"
	block := func(field, why string) {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, p+"."+field, why+": this platform has no equivalent and the import will not weaken isolation")
	}
	if ps.HostNetwork {
		block("hostNetwork", "host networking")
	}
	if ps.HostPID {
		block("hostPID", "host PID namespace")
	}
	if ps.HostIPC {
		block("hostIPC", "host IPC namespace")
	}
	for i, v := range ps.Volumes {
		if v.HostPath != nil {
			block(fmt.Sprintf("volumes[%d].hostPath", i), "hostPath volume "+v.Name)
		}
	}
	all := append(append([]container{}, ps.Containers...), ps.InitContainers...)
	for i, c := range all {
		if c.Security.Privileged != nil && *c.Security.Privileged {
			block(fmt.Sprintf("containers[%d].securityContext.privileged", i), "privileged container "+c.Name)
		}
		if len(c.Security.Capabilities.Add) > 0 {
			block(fmt.Sprintf("containers[%d].securityContext.capabilities.add", i), "added Linux capabilities "+strings.Join(c.Security.Capabilities.Add, ","))
		}
	}
}

func (im *importer) container(o object, a *app, ps podSpec, c container, ws workloadSpec) {
	base := "spec.template.spec.containers[" + c.Name + "]"
	a.image = c.Image
	if a.image == "" {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, base+".image", "no image")
	}
	switch {
	case len(c.Command) > 0:
		a.command = append(append([]string{}, c.Command...), c.Args...)
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".command", "command replaces the image CMD here, the image ENTRYPOINT still runs first")
	case len(c.Args) > 0:
		a.command = append([]string{}, c.Args...)
	}
	a.containerPorts = c.Ports
	for _, p := range c.Ports {
		if p.HostPort > 0 {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".ports.hostPort", fmt.Sprintf("hostPort %d is not bound: use a NodePort or LoadBalancer Service for published ports", p.HostPort))
		}
	}
	im.env(o, a, c, base)
	im.resources(o, a, c, base)
	a.readiness = im.probe(o, a, c.Readiness, base+".readinessProbe")
	a.liveness = im.probe(o, a, c.Liveness, base+".livenessProbe")
	if c.Startup != nil {
		if c.Startup.FailureThreshold > 0 && c.Startup.PeriodSeconds > 0 {
			a.readyTimeout = fmt.Sprintf("%ds", c.Startup.FailureThreshold*c.Startup.PeriodSeconds+c.Startup.InitialDelay)
		}
	}
	im.volumes(o, a, ps, c, ws, base)
}

func (im *importer) env(o object, a *app, c container, base string) {
	setLiteral := func(k, v string) {
		if envredact.SensitiveName(k) || envredact.SensitiveValue(k, v) {
			a.env[k] = envVal{secret: true}
			a.secretKeys[k] = true
			im.res.secrets[a.name+"/"+k] = v
			im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, base+".env."+k, "literal looked like a secret, stored as an app secret")
			return
		}
		if strings.Contains(v, ".svc.cluster.local") || strings.HasSuffix(v, ".svc") {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".env."+k, "value names an in-cluster DNS address: reach another app by its app name here")
		}
		a.env[k] = envVal{literal: v}
	}
	setSecret := func(k, v string) {
		a.env[k] = envVal{secret: true}
		a.secretKeys[k] = true
		im.res.secrets[a.name+"/"+k] = v
	}
	for _, ef := range c.EnvFrom {
		switch {
		case ef.ConfigMapRef != nil:
			key := o.ns() + "/" + ef.ConfigMapRef.Name
			im.usedCM[key] = true
			data, ok := im.configs[key]
			if !ok {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".envFrom", "ConfigMap "+ef.ConfigMapRef.Name+" is not in the input, its keys were not imported")
			}
			for _, k := range sortedKeys(data) {
				if name := ef.Prefix + k; envNameRe.MatchString(name) {
					setLiteral(name, data[k])
				} else {
					im.res.add(SeverityChangesBehavior, "ConfigMap", ef.ConfigMapRef.Name, "data."+k, "key is not a valid environment variable name and was skipped")
				}
			}
		case ef.SecretRef != nil:
			key := o.ns() + "/" + ef.SecretRef.Name
			im.usedSec[key] = true
			data, ok := im.secrets[key]
			if !ok {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".envFrom", "Secret "+ef.SecretRef.Name+" is not in the input, set its keys with the secrets command")
			}
			for _, k := range sortedKeys(data) {
				if name := ef.Prefix + k; envNameRe.MatchString(name) {
					setSecret(name, data[k])
				} else {
					im.res.add(SeverityChangesBehavior, "Secret", ef.SecretRef.Name, "data."+k, "key is not a valid environment variable name and was skipped")
				}
			}
		}
	}
	for _, e := range c.Env {
		f := base + ".env." + e.Name
		switch {
		case e.ValueFrom == nil:
			setLiteral(e.Name, e.Value)
		case e.ValueFrom.ConfigMapKeyRef != nil:
			r := e.ValueFrom.ConfigMapKeyRef
			im.usedCM[o.ns()+"/"+r.Name] = true
			v, ok := im.configs[o.ns()+"/"+r.Name][r.Key]
			if !ok {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, fmt.Sprintf("ConfigMap %s key %s is not in the input, variable omitted", r.Name, r.Key))
				continue
			}
			setLiteral(e.Name, v)
		case e.ValueFrom.SecretKeyRef != nil:
			r := e.ValueFrom.SecretKeyRef
			im.usedSec[o.ns()+"/"+r.Name] = true
			v, ok := im.secrets[o.ns()+"/"+r.Name][r.Key]
			if !ok {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, fmt.Sprintf("Secret %s key %s is not in the input, variable omitted: set it with the secrets command", r.Name, r.Key))
				continue
			}
			setSecret(e.Name, v)
		default:
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, "downward API values are not available, variable omitted")
		}
	}
}

func (im *importer) resources(o object, a *app, c container, base string) {
	pick := func(res, key string) (float64, bool) {
		raw, ok := map[string]map[string]string{"limits": c.Resources.Limits, "requests": c.Resources.Requests}[res][key]
		if !ok {
			return 0, false
		}
		v, err := parseQuantity(raw)
		if err != nil {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".resources."+res+"."+key, "quantity could not be read and was ignored")
			return 0, false
		}
		return v, true
	}
	for _, key := range []string{"memory", "cpu"} {
		v, ok := pick("limits", key)
		if !ok {
			if v, ok = pick("requests", key); ok {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".resources.requests."+key, "only a request was set, applied as the limit")
			}
		} else if _, has := pick("requests", key); has {
			im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, base+".resources.requests."+key, "no reservation concept, the limit is applied")
		}
		if !ok {
			continue
		}
		if key == "memory" {
			a.memory = memoryString(v)
		} else {
			a.cpu = cpuValue(v)
		}
	}
	for _, res := range []map[string]string{c.Resources.Limits, c.Resources.Requests} {
		for k := range res {
			switch {
			case k == "memory" || k == "cpu":
			case strings.HasSuffix(k, "/gpu"):
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, base+".resources."+k, "GPU requests are not applied by the import: set resources.gpu on the app afterwards")
			default:
				im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, base+".resources."+k, "resource is not enforced")
			}
		}
	}
}

func (im *importer) probe(o object, a *app, p *probeIn, field string) *probeOut {
	if p == nil {
		return nil
	}
	out := &probeOut{failures: p.FailureThreshold}
	if p.PeriodSeconds > 0 {
		out.interval = fmt.Sprintf("%ds", p.PeriodSeconds)
	}
	if p.TimeoutSeconds > 0 {
		out.timeout = fmt.Sprintf("%ds", p.TimeoutSeconds)
	}
	switch {
	case p.HTTPGet != nil:
		out.path = p.HTTPGet.Path
		if out.path == "" {
			out.path = "/"
		}
		if strings.EqualFold(p.HTTPGet.Scheme, "https") {
			out.scheme = "https"
		}
		if p.HTTPGet.Port.Str != "" || p.HTTPGet.Port.Int > 0 {
			a.probePorts = append(a.probePorts, p.HTTPGet.Port)
		}
	case p.Exec != nil && len(p.Exec.Command) > 0:
		out.exec = p.Exec.Command
	case p.TCPSocket != nil:
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, field+".tcpSocket", "TCP probes do not exist here: only an HTTP path or an exec command can be checked")
		return nil
	default:
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, field, "probe type is not supported and was ignored")
		return nil
	}
	return out
}

func (im *importer) volumes(o object, a *app, ps podSpec, c container, ws workloadSpec, base string) {
	byName := map[string]podVolume{}
	for _, v := range ps.Volumes {
		byName[v.Name] = v
	}
	claims := map[string]bool{}
	for _, t := range ws.VolumeClaimTemplates {
		claims[t.Metadata.Name] = true
		var pvcSpec struct {
			Resources struct {
				Requests map[string]string `yaml:"requests"`
			} `yaml:"resources"`
		}
		_ = t.Spec.Decode(&pvcSpec)
		im.res.add(SeverityIgnoredSafe, "PersistentVolumeClaim", t.Metadata.Name, "spec.resources.requests.storage", "named volumes have no size request, the host disk is used")
	}
	for _, m := range c.VolumeMounts {
		f := base + ".volumeMounts[" + m.Name + "]"
		if m.SubPath != "" {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f+".subPath", "subPath mounts are not supported, the whole volume is mounted")
		}
		if m.ReadOnly {
			im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, f+".readOnly", "named volumes are mounted read-write")
		}
		v, declared := byName[m.Name]
		switch {
		case claims[m.Name]:
			a.volumes = append(a.volumes, volOut{name: volName(m.Name), path: m.MountPath})
		case declared && v.PVC != nil:
			im.usedPVC[o.ns()+"/"+v.PVC.ClaimName] = true
			if _, ok := im.pvcs[o.ns()+"/"+v.PVC.ClaimName]; !ok {
				im.res.add(SeverityIgnoredSafe, "PersistentVolumeClaim", v.PVC.ClaimName, "", "claim is not in the input, a new empty named volume is created")
			}
			a.volumes = append(a.volumes, volOut{name: volName(v.PVC.ClaimName), path: m.MountPath})
		case declared && v.EmptyDir != nil:
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, "emptyDir becomes a persistent named volume: data now survives restarts")
			a.volumes = append(a.volumes, volOut{name: volName(m.Name), path: m.MountPath})
		case declared && v.HostPath != nil:
			// reported as a blocker by hardening
		case declared && v.ConfigMap != nil:
			im.usedCM[o.ns()+"/"+v.ConfigMap.Name] = true
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, "ConfigMap "+v.ConfigMap.Name+" mounted as files is not imported: stack documents carry no inline files yet, pass settings as env")
		case declared && v.Secret != nil:
			im.usedSec[o.ns()+"/"+v.Secret.SecretName] = true
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, "Secret "+v.Secret.SecretName+" mounted as files is not imported: pass its keys as secret env")
		default:
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, f, "volume type is not supported and was not mounted")
		}
	}
	if len(a.volumes) > 0 && a.replicas > 1 {
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.replicas", "each replica had its own claim; named volumes here are scoped to the service, so replicas share one")
	}
}

func volName(s string) string {
	n := nameCleanR.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(n, "-")
}

func (im *importer) cronJob(o object, a *app, ws workloadSpec, c container) {
	cmd := append(append([]string{}, c.Command...), c.Args...)
	if len(cmd) == 0 {
		im.res.add(SeverityBlocker, o.Kind, o.Metadata.Name, "spec.jobTemplate", "scheduled jobs need an explicit command or args: the image default cannot be scheduled")
		return
	}
	j := jobOut{name: volName(o.Metadata.Name), schedule: ws.Schedule, tz: ws.TimeZone, command: cmd, disabled: ws.Suspend}
	switch strings.ToLower(ws.ConcurrencyPolicy) {
	case "allow", "forbid", "replace":
		j.concurrency = strings.ToLower(ws.ConcurrencyPolicy)
	}
	if d := ws.JobTemplate.Spec.ActiveDeadline; d != nil && *d > 0 {
		j.timeout = fmt.Sprintf("%ds", *d)
	}
	a.jobs = append(a.jobs, j)
	im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "kind", "the app also runs the command once when deployed, then the schedule runs it in fresh containers")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
