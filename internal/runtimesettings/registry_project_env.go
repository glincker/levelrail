package runtimesettings

import (
	"fmt"
	"regexp"
)

// Keys for the environment a new project starts with.
const (
	KeyProjectsDefaultEnvironment     = "projects.default_environment"
	KeyProjectsDefaultEnvironmentName = "projects.default_environment_name"
)

var defaultEnvironmentNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)

func defaultEnvironmentNameCheck(v string) error {
	if !defaultEnvironmentNameRe.MatchString(v) {
		return fmt.Errorf("must be 1 to 63 letters, digits, spaces, dots, dashes or underscores, starting with a letter or digit")
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyProjectsDefaultEnvironment, Category: CategoryDeploys, Kind: KindBool, Default: "false",
		Env: "APP_PROJECTS_DEFAULT_ENVIRONMENT", Apply: ApplyLive,
		Title:       "Projects: create a default environment",
		Description: "When on, a new project starts with one unprotected default environment, and an app or database created in a project without naming an environment joins it. The next project uses a changed value.",
	})
	Register(Definition{
		Key: KeyProjectsDefaultEnvironmentName, Category: CategoryDeploys, Kind: KindString, Default: "production",
		Env: "APP_PROJECTS_DEFAULT_ENVIRONMENT_NAME", Apply: ApplyLive, CheckValue: defaultEnvironmentNameCheck,
		Title:       "Projects: default environment name",
		Description: "Name given to the default environment of a new project. The next project uses a changed value.",
	})
}
