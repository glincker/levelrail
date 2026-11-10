package spec

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

const (
	envUserMaxLen           = "APP_SERVICE_USER_MAX_LEN"
	envWorkingDirMaxLen     = "APP_SERVICE_WORKING_DIR_MAX_LEN"
	defaultUserMaxLen       = 64
	defaultWorkingDirMaxLen = 1024
)

// userPattern is a user or group name or numeric id, optionally name:group.
// It admits no whitespace or shell characters.
var userPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*(:[A-Za-z0-9_][A-Za-z0-9_.-]*)?$`)

// ValidateUser checks a container user value. Errors never echo the value.
func ValidateUser(user string) error {
	if user == "" {
		return nil
	}
	if limit := envPositive(envUserMaxLen, defaultUserMaxLen); len(user) > limit {
		return fmt.Errorf("user: longer than %d characters", limit)
	}
	if !userPattern.MatchString(user) {
		return errors.New("user: must be a name or uid, optionally followed by :group or :gid")
	}
	return nil
}

// ValidateWorkingDir checks a container working directory. Errors never echo the value.
func ValidateWorkingDir(dir string) error {
	if dir == "" {
		return nil
	}
	if limit := envPositive(envWorkingDirMaxLen, defaultWorkingDirMaxLen); len(dir) > limit {
		return fmt.Errorf("working_dir: longer than %d characters", limit)
	}
	if strings.ContainsAny(dir, "\x00\r\n") {
		return errors.New("working_dir: must not contain NUL or line breaks")
	}
	if !path.IsAbs(dir) {
		return errors.New("working_dir: must be an absolute path")
	}
	return nil
}

func (svc Service) validateProcess(name string) error {
	if err := ValidateUser(svc.User); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	if err := ValidateWorkingDir(svc.WorkingDir); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	return nil
}
