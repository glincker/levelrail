package datamigrate

import (
	"errors"
	"regexp"
	"strings"
)

// VolumeRef is one persistent mount of an imported app.
type VolumeRef struct {
	// Name is the Docker volume name here, empty for a bind mount.
	Name          string
	ContainerPath string
	HostPath      string
	// SourceName is the volume's real name on the source host, when it is
	// not derivable from Name by trimming the app prefix.
	SourceName string
}

// VolumeGuide is a command the operator runs on this node to copy one volume.
// Volume contents are not moved by the control plane: it would need root on
// the source host, which is the operator's call to make.
type VolumeGuide struct {
	App           string `json:"app"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	ContainerPath string `json:"container_path"`
	Command       string `json:"command"`
}

var sshTargetRe = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// ValidateSSHTarget accepts only user@host.
func ValidateSSHTarget(s string) error {
	if !sshTargetRe.MatchString(s) {
		return errors.New("source must look like user@host")
	}
	return nil
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// GuideVolumes builds one rsync command per volume, to run as root on the node
// that hosts app. appPrefix is the prefix imports put on volume names
// ("app-<app>-"), so the source volume name is recovered from it.
func GuideVolumes(app, appPrefix, sourceSSH string, vols []VolumeRef) []VolumeGuide {
	out := make([]VolumeGuide, 0, len(vols))
	for _, v := range vols {
		g := VolumeGuide{App: app, ContainerPath: v.ContainerPath}
		if v.HostPath != "" {
			g.Kind, g.Source, g.Target = "bind", v.HostPath, v.HostPath
			g.Command = "mkdir -p " + shQuote(v.HostPath) + " && rsync -aHAX --numeric-ids --delete --rsync-path='sudo rsync' -e ssh " +
				shQuote(sourceSSH+":"+strings.TrimSuffix(v.HostPath, "/")+"/") + " " + shQuote(strings.TrimSuffix(v.HostPath, "/")+"/")
			out = append(out, g)
			continue
		}
		src := strings.TrimPrefix(v.Name, appPrefix)
		if v.SourceName != "" {
			src = v.SourceName
		}
		g.Kind, g.Source, g.Target = "volume", src, v.Name
		g.Command = "docker volume create " + shQuote(v.Name) + " >/dev/null && rsync -aHAX --numeric-ids --delete --rsync-path='sudo rsync' -e ssh " +
			shQuote(sourceSSH+":/var/lib/docker/volumes/"+src+"/_data/") + ` "$(docker volume inspect -f '{{.Mountpoint}}' ` + shQuote(v.Name) + `)/"`
		out = append(out, g)
	}
	return out
}
