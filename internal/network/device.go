package network

// This file: the real Mesh, on wireguard-go (ADR 006's userspace path).
//
// Scope boundary, stated up front rather than discovered by a reader
// halfway down: this type owns the WireGuard device, which means keys,
// peers, the listen port, and the encrypted transport between nodes. It
// does NOT assign the mesh IP to the interface or add routes for it.
// That is a netlink operation on Linux and a route(8)/ioctl operation on
// Darwin, it is what wg-quick(8) does around wireguard-go rather than
// something wireguard-go does, and doing it properly means either a new
// netlink dependency or a few hundred lines of platform-specific,
// root-only, untestable syscall marshalling. Neither belongs in the same
// change as the mesh's decision logic.
//
// So the seam is explicit: LinkConfigurator. Supply one and the device is
// fully usable; supply none and the WireGuard device still comes up and
// still handshakes, but nothing routes through it until the interface is
// addressed externally, and the device says so loudly (once, at Warn,
// naming the interface and the address it needs) instead of reporting a
// success that is not one. An unaddressed interface that silently
// reported healthy is exactly the class of failure that sets a deploy
// platform's reputation: the ones a health check cannot catch because
// it never looked.
//
// Both backends land here. The in-kernel path (BackendKernel) is
// currently detected but not separately implemented: Detect will report
// kernel, and this device runs wireguard-go anyway, which is correct but
// slower than it needs to be. See NewDevice for why that is the honest
// state rather than a claim of kernel support.

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// LinkConfigurator assigns a mesh address to the network interface the
// WireGuard device was created on, and brings it up. See this file's
// header for why it is a seam rather than an implementation.
//
// Implementations must be idempotent: Apply calls SetAddress on every
// reconcile pass with the same address, the same level-triggered contract
// Mesh.Apply itself has.
type LinkConfigurator interface {
	// SetAddress assigns addr to the interface named iface and brings the
	// interface up.
	SetAddress(ctx context.Context, iface string, addr netip.Prefix) error
}

// tunDevice is the slice of tun.Device this package uses, extracted as an
// interface so Device's lifecycle (create, configure, close) can be
// tested against a fake without a real TUN device, which needs root.
type tunDevice interface {
	Name() (string, error)
	Close() error
}

// wgDevice is the slice of *device.Device this package uses, same
// reasoning as tunDevice.
type wgDevice interface {
	IpcSet(uapiConf string) error
	IpcGet() (string, error)
	Up() error
	Close()
}

// Device is a Mesh backed by a real WireGuard device.
type Device struct {
	backend Backend
	iface   string
	logger  *slog.Logger
	link    LinkConfigurator

	// dev owns the TUN device it was constructed with: wireguard-go's
	// own Close closes it, so this type deliberately does not keep a
	// second reference to the TUN and never closes it twice.
	dev wgDevice

	mu           sync.Mutex
	closed       bool
	lastCfg      DeviceConfig
	warnedNoLink bool
}

// DeviceOption configures NewDevice.
type DeviceOption func(*deviceOptions)

type deviceOptions struct {
	logger    *slog.Logger
	link      LinkConfigurator
	mtu       int
	shortName string
	newTUN    func(name string, mtu int) (tunDevice, error)
	newWG     func(t tunDevice, logger *slog.Logger, iface string) wgDevice
	backend   Backend
}

// WithLogger sets the structured logger. Defaults to slog.Default().
func WithLogger(l *slog.Logger) DeviceOption {
	return func(o *deviceOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithLinkConfigurator supplies the interface addressing step this
// package deliberately does not implement. See this file's header.
func WithLinkConfigurator(lc LinkConfigurator) DeviceOption {
	return func(o *deviceOptions) { o.link = lc }
}

// WithMTU overrides the TUN device MTU. Defaults to wireguard-go's own
// DefaultMTU (1420), which is 1500 minus WireGuard's worst-case
// encapsulation overhead; lowering it is a real operator need on links
// that are themselves tunnelled (PPPoE, some VPS providers), where the
// default produces silent fragmentation-related stalls rather than an
// error.
func WithMTU(mtu int) DeviceOption {
	return func(o *deviceOptions) {
		if mtu > 0 {
			o.mtu = mtu
		}
	}
}

// WithShortName sets the brand short name the interface name is derived
// from (the product name never appears in source, so this is passed in,
// not looked up). Callers pass brand.Brand.ShortName.
func WithShortName(s string) DeviceOption {
	return func(o *deviceOptions) { o.shortName = s }
}

// NewDevice creates a WireGuard device for this node.
//
// The backend is chosen by Detect, and a BackendDisabled result is
// returned as a *Disabled rather than an error: a node that cannot mesh
// must still run (see Detect's doc comment for why).
//
// A BackendKernel result currently still runs wireguard-go. That is a
// real, deliberate gap and not an oversight: driving the in-kernel module
// means creating the interface over netlink (RTM_NEWLINK with a
// "wireguard" link kind) and configuring it over the wireguard generic
// netlink family, which is a second, Linux-only implementation of
// everything in this file plus its own dependency. Detection is built
// now, per ADR 006 and TASKS.md 3.4, because the decision point and its
// reasoning are what a later change needs in place; the second backend
// itself is worth landing on its own, against a real Linux node, rather
// than written blind here. Status reports the backend actually in use,
// so this never claims otherwise.
func NewDevice(ctx context.Context, probe Probe, opts ...DeviceOption) (Mesh, error) {
	o := deviceOptions{
		logger:    slog.Default(),
		mtu:       device.DefaultMTU,
		shortName: "",
		newTUN:    createSystemTUN,
		newWG:     newSystemWGDevice,
	}
	for _, opt := range opts {
		opt(&o)
	}

	detected := Detect(probe)
	if o.backend != "" {
		detected.Backend = o.backend
	}
	iface := interfaceName(o.shortName)

	if detected.Backend == BackendDisabled {
		o.logger.Warn("mesh networking disabled on this node",
			slog.String("interface", iface), slog.String("reason", detected.Reason))
		return NewDisabled(detected.Reason), nil
	}

	o.logger.Info("bringing up mesh device",
		slog.String("interface", iface),
		slog.String("backend", string(detected.Backend)),
		slog.String("reason", detected.Reason))

	t, err := o.newTUN(iface, o.mtu)
	if err != nil {
		return nil, fmt.Errorf("network: create tun device %q: %w", iface, err)
	}
	// The kernel may hand back a different name than requested (Darwin's
	// utun numbering in particular ignores the requested name entirely),
	// and every later log line and LinkConfigurator call must use the
	// real one, not the one that was asked for.
	if realName, nameErr := t.Name(); nameErr == nil && realName != "" {
		iface = realName
	}

	d := &Device{
		backend: detected.Backend,
		iface:   iface,
		logger:  o.logger,
		link:    o.link,
		dev:     o.newWG(t, o.logger, iface),
	}
	if err := d.dev.Up(); err != nil {
		// Close the TUN we already created rather than leaking an
		// interface on a failed startup.
		if cerr := t.Close(); cerr != nil {
			o.logger.Error("closing tun device after failed startup",
				slog.String("interface", iface), slog.String("error", cerr.Error()))
		}
		return nil, fmt.Errorf("network: bring up wireguard device %q: %w", iface, err)
	}
	if err := ctx.Err(); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("network: create mesh device %q: %w", iface, err)
	}
	return d, nil
}

// Apply converges the device on cfg. See EncodeUAPI for why this reads
// the device's current peers first instead of replacing them wholesale.
func (d *Device) Apply(ctx context.Context, cfg DeviceConfig) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("network: apply mesh config for node %q: %w", cfg.NodeID, err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("network: apply mesh config for node %q: %w", cfg.NodeID, ErrMeshClosed)
	}

	raw, err := d.dev.IpcGet()
	if err != nil {
		return fmt.Errorf("network: read current state of device %q for node %q: %w", d.iface, cfg.NodeID, err)
	}
	current, err := ParseUAPIStatus(raw, nil)
	if err != nil {
		return fmt.Errorf("network: parse current state of device %q for node %q: %w", d.iface, cfg.NodeID, err)
	}

	if err := d.dev.IpcSet(EncodeUAPI(cfg, peerKeys(current))); err != nil {
		return fmt.Errorf("network: configure device %q for node %q: %w", d.iface, cfg.NodeID, err)
	}

	if err := d.configureLink(ctx, cfg); err != nil {
		return err
	}

	d.lastCfg = cfg
	d.logger.Debug("mesh config applied",
		slog.String("interface", d.iface), slog.Any("config", cfg))
	return nil
}

// configureLink runs the interface addressing step, or explains exactly
// once why it did not. Called with d.mu held.
func (d *Device) configureLink(ctx context.Context, cfg DeviceConfig) error {
	if !cfg.Address.IsValid() {
		// No address assigned to this node yet: a normal first-pass
		// state (the control plane allocates one and sends it on a later
		// pass), not something to warn about.
		return nil
	}
	if d.link == nil {
		if !d.warnedNoLink {
			d.warnedNoLink = true
			d.logger.Warn("mesh interface has no address configured: inter-node traffic will not route until it does",
				slog.String("interface", d.iface),
				slog.String("node_id", cfg.NodeID),
				slog.String("required_address", cfg.Address.String()))
		}
		return nil
	}
	if err := d.link.SetAddress(ctx, d.iface, cfg.Address); err != nil {
		return fmt.Errorf("network: assign %s to interface %q for node %q: %w",
			cfg.Address, d.iface, cfg.NodeID, err)
	}
	return nil
}

// Status reads the device's live state.
func (d *Device) Status(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, fmt.Errorf("network: mesh status for device %q: %w", d.iface, err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return Status{}, fmt.Errorf("network: mesh status for device %q: %w", d.iface, ErrMeshClosed)
	}

	raw, err := d.dev.IpcGet()
	if err != nil {
		return Status{}, fmt.Errorf("network: read state of device %q: %w", d.iface, err)
	}
	st, err := ParseUAPIStatus(raw, d.nodeIDsByKeyLocked())
	if err != nil {
		return Status{}, fmt.Errorf("network: parse state of device %q: %w", d.iface, err)
	}
	st.Backend = d.backend
	st.Interface = d.iface
	st.Address = d.lastCfg.Address
	return st, nil
}

// nodeIDsByKeyLocked builds the public-key-to-node-ID map ParseUAPIStatus
// needs, from the last config applied. Called with d.mu held.
func (d *Device) nodeIDsByKeyLocked() map[Key]string {
	m := make(map[Key]string, len(d.lastCfg.Peers))
	for _, p := range d.lastCfg.Peers {
		m[p.PublicKey] = p.NodeID
	}
	return m
}

// Close tears the device down. Idempotent.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.dev.Close() // wireguard-go's Close closes the TUN it was given
	return nil
}

// createSystemTUN is the real TUN constructor, split out so NewDevice's
// own logic is testable against a fake.
func createSystemTUN(name string, mtu int) (tunDevice, error) {
	t, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// newSystemWGDevice builds the real wireguard-go device, routing its logs
// into this codebase's structured logger rather than wireguard-go's own
// stdout writer (structured logging with log/slog, and every log line
// about a resource carries its ID, which here is the interface).
func newSystemWGDevice(t tunDevice, logger *slog.Logger, iface string) wgDevice {
	systemTUN, ok := t.(tun.Device)
	if !ok {
		// Only reachable if a caller injected a fake TUN through the
		// unexported option and then asked for the real device
		// constructor, which no code path does.
		panic("network: newSystemWGDevice requires a real tun.Device")
	}
	return device.NewDevice(systemTUN, conn.NewDefaultBind(), &device.Logger{
		Verbosef: func(format string, args ...any) {
			logger.Debug(fmt.Sprintf(format, args...), slog.String("interface", iface))
		},
		Errorf: func(format string, args ...any) {
			logger.Error(fmt.Sprintf(format, args...), slog.String("interface", iface))
		},
	})
}
