package runtimesettings

// Password policy keys. The defaults match the rule that shipped before these settings existed.
const (
	KeyAuthPasswordMinLength      = "auth.password_min_length"
	KeyAuthPasswordRequireClasses = "auth.password_require_classes"
)

// PasswordMinLengthFloor is the weakest length any policy may reach, enforced in code as well.
const PasswordMinLengthFloor = 8

const passwordMinLengthCeiling = 128

func init() {
	Register(Definition{
		Key: KeyAuthPasswordMinLength, Category: CategorySecurity, Kind: KindInt, Default: "8",
		Apply: ApplyLive, CheckValue: intRangeCheck(PasswordMinLengthFloor, passwordMinLengthCeiling),
		Title:       "Minimum password length",
		Description: "Fewest characters a new password may have, from 8 to 128. Applies to sign-up, invites, password reset and password change. Existing passwords keep working until they are changed.",
	})
	Register(Definition{
		Key: KeyAuthPasswordRequireClasses, Category: CategorySecurity, Kind: KindInt, Default: "0",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 4),
		Title:       "Required character types",
		Description: "How many of the four character types (lowercase, uppercase, digit, symbol) a new password must mix, from 0 to 4. 0 asks for no mix. Applies to sign-up, invites, password reset and password change.",
	})
}
