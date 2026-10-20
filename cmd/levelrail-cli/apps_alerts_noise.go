package main

import "flag"

// alertNoiseFlags are the per-rule noise control flags shared by
// "apps alerts create" and "apps alerts update".
type alertNoiseFlags struct {
	severity, flapWindow string
	consecutive, flapMax int
	labels               stringList
}

const alertNoiseFlagsUsage = `  --severity string                    info, warning (default) or critical; used by silences and shown in history
  --consecutive-failures int           evaluation ticks the condition must hold in a row before firing (default: APP_ALERT_CONSECUTIVE_FAILURES)
  --flap-threshold int                 fires within --flap-window that mark the rule flapping (default: APP_ALERT_FLAP_THRESHOLD)
  --flap-window string                 window for --flap-threshold, e.g. "30m" (default: APP_ALERT_FLAP_WINDOW)
  --label key=value                    label a silence can match (repeatable)
`

func registerAlertNoiseFlags(fs *flag.FlagSet) *alertNoiseFlags {
	n := &alertNoiseFlags{}
	fs.StringVar(&n.severity, "severity", "", "info, warning or critical")
	fs.IntVar(&n.consecutive, "consecutive-failures", 0, "ticks the condition must hold in a row before firing")
	fs.IntVar(&n.flapMax, "flap-threshold", 0, "fires within --flap-window that mark the rule flapping")
	fs.StringVar(&n.flapWindow, "flap-window", "", "window for --flap-threshold, e.g. \"30m\"")
	fs.Var(&n.labels, "label", "label a silence can match, key=value (repeatable)")
	return n
}

func (n *alertNoiseFlags) apply(req *createAlertRuleRequest) error {
	labels, err := labelMap(n.labels)
	if err != nil {
		return err
	}
	req.Severity, req.Labels = n.severity, labels
	req.ConsecutiveFailures, req.FlapThreshold, req.FlapWindow = n.consecutive, n.flapMax, n.flapWindow
	return nil
}
