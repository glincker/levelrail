package apptype

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/policy"
)

const fixtureDoc = `{
  "name": "internal-http-api", "workload": "web", "enforcement": "enforce",
  "parameters": {"type": "object", "required": ["port"], "properties": {
    "port": {"type": "integer", "minimum": 1024, "maximum": 65535, "default": 8080},
    "tier": {"type": "string", "enum": ["small", "medium"], "default": "small"}}},
  "binds": {"port": "services.*.port", "tier": "resources"},
  "environmentConfig": {"production": {"replicasMin": 2, "readinessRequired": true}},
  "build": {"allowed": ["railpack", "dockerfile"]},
  "addons": {"databases": {"allowed_engines": ["postgres"], "max": 1}, "volumes": {"allowed": false}},
  "fixed": {"labels.team": "required", "egress": "private"}
}`

func TestSchemaSubsetAcceptsAndRejects(t *testing.T) {
	lim := Limits{MaxParams: 2, MaxSchemaBytes: 4096, MaxPatternLen: 8, MaxVersions: 5}
	good := `{"type":"object","properties":{"a":{"type":"string","pattern":"^a+$","maxLength":4},"b":{"type":"boolean"}}}`
	if _, err := ParseSchema([]byte(good), lim); err != nil {
		t.Fatalf("good schema rejected: %v", err)
	}
	bad := map[string]string{
		`{"type":"object","properties":{"a":{"$ref":"#/x"}}}`:                                                   "keyword \"$ref\" is not supported",
		`{"type":"object","oneOf":[]}`:                                                                          "oneOf",
		`{"type":"object","properties":{"a":{"type":"string","pattern":"^(a+)+$aaaaaa"}}}`:                      "pattern is",
		`{"type":"object","properties":{"a":{"type":"string","pattern":"(?=x)"}}}`:                              "RE2",
		`{"type":"object","properties":{"a":{"type":"boolean"},"b":{"type":"boolean"},"c":{"type":"boolean"}}}`: "limit is 2",
		`{"type":"object","additionalProperties":true}`:                                                         "additionalProperties must be false",
		`{"type":"object","properties":{"a":{"type":"integer","minLength":1}}}`:                                 "only applies",
		`{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"object"}}}}}`:           "one level",
		`{"type":"object","properties":{"a":{"type":"integer","default":"x"}}}`:                                 "whole number",
	}
	for doc, want := range bad {
		if _, err := ParseSchema([]byte(doc), lim); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", doc, err, want)
		}
	}
	tiny := Limits{MaxParams: 2, MaxSchemaBytes: 10, MaxPatternLen: 8}
	if _, err := ParseSchema([]byte(good), tiny); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Errorf("size cap not enforced: %v", err)
	}
}

func TestCheckReportsReadableViolations(t *testing.T) {
	ty, err := ParseDocument([]byte(fixtureDoc), LimitsFromEnv())
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	spec := Spec{
		EnvKind: "production", Build: "static", Replicas: &one,
		Params:    map[string]any{"port": float64(80), "tier": "huge", "extra": true},
		Databases: []string{"mysql", "postgres"}, Volumes: 1,
		Values: map[string]string{"egress": "public"},
	}
	got := map[string]policy.Violation{}
	for _, v := range Check(ty, spec) {
		got[v.Code+"|"+v.Rule] = v
		if v.Mode != policy.ModeEnforce || v.Message == "" || v.NextAction == "" {
			t.Errorf("incomplete violation %+v", v)
		}
	}
	for _, want := range []string{
		CodeParamInvalid + "|parameters", CodeBuildNotAllowed + "|build.allowed", CodeAddonNotAllowed + "|addons.databases",
		CodeAddonNotAllowed + "|addons.volumes", CodeFixedValueMissing + "|fixed.labels.team",
		CodeFixedValueChanged + "|fixed.egress", CodeReplicasOutOfRange + "|environment.production.replicas",
		CodeReadinessRequired + "|environment.production.readiness",
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing violation %s in %v", want, got)
		}
	}
	if m := got[CodeBuildNotAllowed+"|build.allowed"].Message; !strings.Contains(m, "railpack, dockerfile") {
		t.Errorf("build message = %q", m)
	}
}

func TestCheckCleanSpecAndUnboundParam(t *testing.T) {
	ty, err := ParseDocument([]byte(fixtureDoc), LimitsFromEnv())
	if err != nil {
		t.Fatal(err)
	}
	two := 2
	ok := Spec{EnvKind: "production", Build: "railpack", Replicas: &two, Readiness: true,
		Params: map[string]any{"port": float64(9000)}, Databases: []string{"postgres"},
		Values: map[string]string{"labels.team": "core", "egress": "private"}}
	if v := Check(ty, ok); len(v) != 0 {
		t.Fatalf("clean spec has violations: %+v", v)
	}
	unbound := strings.Replace(fixtureDoc, `, "tier": "resources"`, "", 1)
	if _, err := ParseDocument([]byte(unbound), LimitsFromEnv()); err == nil || !strings.Contains(err.Error(), "no entry in binds") {
		t.Fatalf("unbound parameter accepted: %v", err)
	}
}
