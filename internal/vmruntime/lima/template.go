package lima

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/vmruntime/guest"
)

// Pinned Ubuntu 26.04 LTS cloud images. Digests are the publisher's SHA-256;
// a bump ships with a real-Mac isolation check.
const (
	imageBase     = "https://cloud-images.ubuntu.com/releases/resolute/release-20260927/"
	imageARM64SHA = "63a93bd5a8d76e33b15ceb5daa3657bd79be804748051ab178e643b0f5da22e7"
	imageAMD64SHA = "8800651811af9a85465ad1d552add729947bb16488dddb4a9b5305a3d97332b2"
	// ImageSHAEnv overrides the digest of the image for the chosen arch. TESTING
	// only: production code must not set it.
	ImageSHAEnv = "APP_VM_IMAGE_SHA256"

	loopback         = "127.0.0.1"
	minLimaVersion   = "2.0.0"
	archARM64        = "aarch64"
	archAMD64        = "x86_64"
	portRangeLo      = 1
	portRangeHi      = 65535
	guestMemoryUnit  = "MiB"
	guestDiskUnit    = "GiB"
	provisionModeSys = "system"
)

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// RenderOptions carries caller choices that are not VM settings.
type RenderOptions struct {
	// Arch is "aarch64" or "x86_64". Empty means aarch64.
	Arch string
	// ImageSHA256Override replaces the pinned digest for Arch. Testing only.
	ImageSHA256Override string
}

// ImageOverrideFromEnv reads APP_VM_IMAGE_SHA256 for RenderOptions. Testing only.
func ImageOverrideFromEnv() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv(ImageSHAEnv)))
}

// Image is one pinned guest image entry.
type Image struct {
	Location string `yaml:"location"`
	Arch     string `yaml:"arch"`
	Digest   string `yaml:"digest"`
}

type limaMount struct {
	Location   string `yaml:"location"`
	MountPoint string `yaml:"mountPoint"`
	Writable   bool   `yaml:"writable"`
}

type limaForward struct {
	GuestPort      int    `yaml:"guestPort,omitempty"`
	GuestPortRange []int  `yaml:"guestPortRange,omitempty"`
	HostPort       int    `yaml:"hostPort,omitempty"`
	HostIP         string `yaml:"hostIP,omitempty"`
	Static         bool   `yaml:"static,omitempty"`
	Ignore         bool   `yaml:"ignore,omitempty"`
}

type limaProvision struct {
	Mode   string `yaml:"mode"`
	Script string `yaml:"script"`
}

type limaToggle struct {
	System bool `yaml:"system"`
	User   bool `yaml:"user"`
}

type limaSSH struct {
	LoadDotSSHPubKeys bool `yaml:"loadDotSSHPubKeys"`
	ForwardAgent      bool `yaml:"forwardAgent"`
	ForwardX11        bool `yaml:"forwardX11"`
}

type limaRosetta struct {
	Enabled bool `yaml:"enabled"`
	Binfmt  bool `yaml:"binfmt"`
}

type limaVMOpts struct {
	VZ struct {
		Rosetta limaRosetta `yaml:"rosetta"`
	} `yaml:"vz"`
}

type limaDevice struct {
	Device string `yaml:"device"`
}

type limaTemplate struct {
	MinimumLimaVersion string          `yaml:"minimumLimaVersion"`
	VMType             string          `yaml:"vmType"`
	CPUs               int             `yaml:"cpus"`
	Memory             string          `yaml:"memory"`
	Disk               string          `yaml:"disk"`
	Images             []Image         `yaml:"images"`
	Plain              bool            `yaml:"plain"`
	MountType          string          `yaml:"mountType,omitempty"`
	Mounts             []limaMount     `yaml:"mounts"`
	Containerd         limaToggle      `yaml:"containerd"`
	SSH                limaSSH         `yaml:"ssh"`
	PortForwards       []limaForward   `yaml:"portForwards"`
	VMOpts             limaVMOpts      `yaml:"vmOpts"`
	Audio              limaDevice      `yaml:"audio"`
	Video              limaDisplay     `yaml:"video"`
	PropagateProxyEnv  bool            `yaml:"propagateProxyEnv"`
	UpgradePackages    bool            `yaml:"upgradePackages"`
	Provision          []limaProvision `yaml:"provision"`
}

type limaDisplay struct {
	Display string `yaml:"display"`
}

// Images returns the pinned images, applying the testing override to arch.
func Images(arch, override string) ([]Image, error) {
	if override != "" && !sha256Re.MatchString(override) {
		return nil, fmt.Errorf("image digest override must be 64 lowercase hex characters")
	}
	arm, amd := imageARM64SHA, imageAMD64SHA
	switch arch {
	case archARM64:
		if override != "" {
			arm = override
		}
	case archAMD64:
		if override != "" {
			amd = override
		}
	default:
		return nil, fmt.Errorf("unsupported arch %q", arch)
	}
	return []Image{
		{Location: imageBase + "ubuntu-26.04-server-cloudimg-arm64.img", Arch: archARM64, Digest: "sha256:" + arm},
		{Location: imageBase + "ubuntu-26.04-server-cloudimg-amd64.img", Arch: archAMD64, Digest: "sha256:" + amd},
	}, nil
}

// Render validates s against l and returns the Lima YAML. Shares need Lima's
// non-plain mode (plain ignores mounts), so a catch-all ignore rule then keeps
// guest agent auto forwarding off.
func Render(s Settings, l Limits, ro RenderOptions) ([]byte, error) {
	if err := s.Validate(l); err != nil {
		return nil, err
	}
	arch := ro.Arch
	if arch == "" {
		arch = archARM64
	}
	images, err := Images(arch, ro.ImageSHA256Override)
	if err != nil {
		return nil, err
	}
	opts := s.Guest
	opts.ExpectedShares = len(s.Shares)
	script, err := guest.ProvisionScript(opts, s.Sidecar)
	if err != nil {
		return nil, err
	}
	t := limaTemplate{
		MinimumLimaVersion: minLimaVersion,
		VMType:             "vz",
		CPUs:               s.CPUs,
		Memory:             fmt.Sprintf("%d%s", s.MemoryMiB, guestMemoryUnit),
		Disk:               fmt.Sprintf("%d%s", s.DiskGiB, guestDiskUnit),
		Images:             images,
		Plain:              len(s.Shares) == 0,
		Mounts:             []limaMount{},
		SSH:                limaSSH{},
		Video:              limaDisplay{Display: "none"},
		PortForwards: []limaForward{
			{GuestPort: s.DashboardGuestPort, HostPort: s.DashboardPort, HostIP: loopback, Static: true},
			{GuestPort: s.IngressGuestPort, HostPort: s.IngressPort, HostIP: loopback, Static: true},
		},
		Provision: []limaProvision{{Mode: provisionModeSys, Script: script}},
	}
	if len(s.Shares) > 0 {
		t.MountType = "virtiofs"
		for _, sh := range s.Shares {
			t.Mounts = append(t.Mounts, limaMount{Location: sh.HostPath, MountPoint: sh.guestPath(), Writable: false})
		}
		t.PortForwards = append(t.PortForwards, limaForward{GuestPortRange: []int{portRangeLo, portRangeHi}, Ignore: true})
	}
	body, err := yaml.Marshal(t)
	if err != nil {
		return nil, err
	}
	header := "# Generated by Levelrail. Edits are overwritten.\n"
	return []byte(header + string(body)), nil
}
