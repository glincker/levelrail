package runtimesettings

// Organization isolation settings. Both default to off so an existing install behaves as before.
const (
	KeyOrgsStrictIsolation = "orgs.strict_isolation"
	KeyOrgsAdminsCanCreate = "orgs.admins_can_create"
	// KeyOrgsProvisionedDefaultMembership defaults on so existing installs keep joining provisioned users to the default org.
	KeyOrgsProvisionedDefaultMembership = "orgs.provisioned_default_membership"
)

func init() {
	Register(Definition{
		Key: KeyOrgsStrictIsolation, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_ORGS_STRICT_ISOLATION", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "Strict organization isolation",
		Description: "Keeps every organization separate from the others. API tokens with no owner and no organization limit are treated as members of the default organization instead of instance admins, and creating an organization needs an instance admin with the root ability. Turn it on once each token is owned by a user or limited to an organization. Turning it off again loosens these checks and asks for confirmation.",
	})
	Register(Definition{
		Key: KeyOrgsAdminsCanCreate, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Apply:       ApplyLive,
		Title:       "Let write admins create organizations",
		Description: "Under strict organization isolation, also let users and tokens with the write ability create organizations. Off means only root can. It has no effect while strict isolation is off, where write is enough already.",
	})
	Register(Definition{
		Key: KeyOrgsProvisionedDefaultMembership, Category: CategorySecurity, Kind: KindBool, Default: "true",
		Apply:       ApplyLive,
		Title:       "Add provisioned users to the default organization",
		Description: "New users created by single sign-on or directory sync join the default organization, which gives instance wide access. Under strict organization isolation, turn this off so they only get the organizations their group mappings grant.",
	})
}
