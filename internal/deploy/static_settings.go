package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/glincker/levelrail/internal/spec"
)

// applyStaticSiteSettings lets the per-site dashboard overrides replace the
// detected build command and publish directory. With no command, the output
// directory picks the subdirectory of the source (git checkout, folder or zip)
// that is served as is.
func (p *Pipeline) applyStaticSiteSettings(ctx context.Context, req *Request) error {
	reader, ok := p.staticSites.(StaticSiteSettingsReader)
	if !ok {
		return nil
	}
	set, _, err := reader.GetStaticSiteSettings(ctx, req.ServiceName)
	if err != nil {
		return fmt.Errorf("read site settings: %w", err)
	}
	if set.BuildCommand == "" && set.OutputDir == "" {
		return nil
	}
	if err := spec.ValidateStaticOverrides(set.BuildCommand, set.OutputDir, spec.StaticBuildCommandMaxFromEnv(os.LookupEnv)); err != nil {
		return err
	}
	b := &req.Service.Build
	if set.BuildCommand != "" {
		b.Command = set.BuildCommand
	}
	switch {
	case set.OutputDir != "" && b.Command != "":
		b.Publish, b.Path = set.OutputDir, ""
	case set.OutputDir != "":
		b.Path = set.OutputDir
	}
	if b.Command != "" && b.Publish == "" {
		if b.Path == "" {
			return errors.New("the build_command override needs an output_dir, none was detected")
		}
		b.Publish, b.Path = b.Path, ""
	}
	return nil
}
