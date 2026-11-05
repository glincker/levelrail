package runtimesettings

// KeyCertificatesWeeklyIssueWarn is the certificate count in seven days at which the budget meter warns.
const KeyCertificatesWeeklyIssueWarn = "certificates.weekly_issue_warn"

const certificatesWeeklyWarnCeiling = 100000

func init() {
	// No env seed: the value is a runtime setting only.
	Register(Definition{
		Key: KeyCertificatesWeeklyIssueWarn, Category: CategoryNetworking, Kind: KindInt, Default: "40",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, certificatesWeeklyWarnCeiling),
		Title:       "Certificates issued per week warning",
		Description: "Certificates this server received from the public CA in the last 7 days at which the certificate budget meter and the doctor show a warning, from 1 to 100000. It only warns, nothing is blocked. The next check uses a changed value.",
	})
}
