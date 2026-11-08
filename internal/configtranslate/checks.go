package configtranslate

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

const fileChecks = "CHECKS"

func checksFinding(code string, sev Severity, subject, msg, fix string) Finding {
	return Finding{Code: code, Severity: sev, Source: fileChecks, Subject: subject, Message: msg, Fix: fix}
}

// checksHealth maps a CHECKS file to a readiness probe. Only the first plain
// path line becomes the probe; every other line yields a finding.
func checksHealth(b []byte) (*Health, []Finding) {
	var fs []Finding
	var probe *Probe
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && !strings.ContainsAny(k, " /") {
			fs = append(fs, checksSetting(&probe, strings.ToUpper(k), v)...)
			continue
		}
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "//"):
			fs = append(fs, checksFinding("checks.host", SeverityWarn, line,
				"a host-prefixed check has no equivalent, so it is skipped",
				"Probe a path on the service itself, or add the check by hand."))
		case !strings.HasPrefix(line, "/"):
			fs = append(fs, checksFinding("checks.unsupported", SeverityWarn, line,
				"only path lines starting with / can be translated, this line is skipped",
				"Add health.readiness by hand, with an exec probe for anything beyond a path."))
		case probe != nil && probe.Path != "":
			fs = append(fs, checksFinding("checks.extra", SeverityInfo, fields[0],
				"a readiness probe checks one path, the first path line is used and this one is skipped",
				"Add an exec probe if this path must also pass."))
		default:
			if probe == nil {
				probe = &Probe{}
			}
			probe.Path = fields[0]
			if len(fields) > 1 {
				fs = append(fs, checksFinding("checks.content", SeverityWarn, fields[0],
					"the expected response text is not checked, the probe passes on a healthy status code",
					"Use an exec probe that greps the response if the text matters."))
			}
		}
	}
	if sc.Err() != nil {
		fs = append(fs, checksFinding("checks.unreadable", SeverityWarn, "",
			"the CHECKS file has a line that is too long to read, the rest is skipped", "Split or shorten the line."))
	}
	if probe == nil || probe.Path == "" {
		fs = append(fs, checksFinding("checks.empty", SeverityInfo, "",
			"the CHECKS file has no path line, so deploys are not readiness gated",
			"Add health.readiness with a cheap path such as /health."))
		return nil, fs
	}
	return &Health{Readiness: probe}, fs
}

func checksSetting(probe **Probe, key, val string) []Finding {
	n, err := strconv.Atoi(strings.TrimSpace(val))
	switch key {
	case "TIMEOUT", "ATTEMPTS":
		if err == nil && n > 0 {
			if *probe == nil {
				*probe = &Probe{}
			}
			if key == "TIMEOUT" {
				(*probe).Timeout = strconv.Itoa(n) + "s"
			} else {
				(*probe).Failures = n
			}
			return nil
		}
		return []Finding{checksFinding("checks.setting.invalid", SeverityWarn, key,
			key+" is not a positive whole number, so it is skipped", "Set health.readiness timeout or failures by hand.")}
	case "WAIT":
		return []Finding{checksFinding("checks.wait", SeverityInfo, key,
			"WAIT (a delay before the first check) is not translated, readiness retries until the container is up",
			"Raise health.readiness failures if the app needs longer to start.")}
	}
	return []Finding{checksFinding("checks.setting.unsupported", SeverityWarn, key,
		key+" is not a CHECKS setting that can be translated, so it is skipped", "Add the matching health option by hand.")}
}
