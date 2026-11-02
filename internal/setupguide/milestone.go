package setupguide

// Milestone names how far an install has come toward its first running app.
// The value doubles as the key the UI translates.
type Milestone string

// First-app milestones, in order. Every one maps to a tracked step except
// install_done, which holds once the guide exists at all.
const (
	MilestoneInstallDone      Milestone = "install_done"
	MilestoneHTTPSOn          Milestone = "https_on"
	MilestoneFirstDomain      Milestone = "first_domain"
	MilestoneFirstAppDeployed Milestone = "first_app_deployed"
	MilestoneFirstBackup      Milestone = "first_backup"
	MilestoneRestoreVerified  Milestone = "restore_verified"
)

var milestoneSteps = []struct {
	milestone Milestone
	step      StepID
}{
	{MilestoneHTTPSOn, StepTLS},
	{MilestoneFirstDomain, StepAppDomain},
	{MilestoneFirstAppDeployed, StepFirstApp},
	{MilestoneFirstBackup, StepFirstBackup},
	{MilestoneRestoreVerified, StepVerifiedBackup},
}

// FirstAppMilestone returns the last milestone reached in order and the one
// after it. next is empty once every milestone is reached.
func FirstAppMilestone(g Guide) (current, next Milestone) {
	states := make(map[StepID]State, len(g.Steps))
	for _, s := range g.Steps {
		states[s.ID] = s.State
	}
	current = MilestoneInstallDone
	for _, m := range milestoneSteps {
		// Not applicable (for example TLS handled upstream) counts as reached.
		if st := states[m.step]; st != StateDone && st != StateNotApplicable {
			return current, m.milestone
		}
		current = m.milestone
	}
	return current, ""
}
