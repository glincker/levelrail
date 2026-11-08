package cdn

// Step statuses, the same words the doctor checks use.
const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusFail    = "fail"
	StatusSkipped = "skipped"
)

// Setup step ids, in run order.
const (
	StepProvider = "provider"
	StepSSLMode  = "ssl_mode"
	StepDNS01    = "acme_dns01"
	StepClientIP = "client_address"
	StepDNSPlan  = "dns_plan"
	StepApply    = "apply"
	StepVerify   = "verify"
)

// Step is one line of the setup checklist.
type Step struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	// Fix is the command that resolves a warn or fail, empty when none is needed.
	Fix string `json:"fix,omitempty"`
}

// SetupResult is the answer to a setup run, usable as a wizard's state.
type SetupResult struct {
	App      string `json:"app"`
	Provider string `json:"provider"`
	DryRun   bool   `json:"dry_run"`
	Applied  bool   `json:"applied"`
	Status   string `json:"status"`
	Steps    []Step `json:"steps"`
}

// Summarize sets Status to the worst step status: fail, then warn, else ok.
func (r *SetupResult) Summarize() {
	r.Status = StatusOK
	for _, s := range r.Steps {
		switch s.Status {
		case StatusFail:
			r.Status = StatusFail
			return
		case StatusWarn:
			r.Status = StatusWarn
		}
	}
}
