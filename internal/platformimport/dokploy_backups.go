package platformimport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// BackupDestination is an object-storage target read from the source. It never holds the
// access keys: those are read only at apply time through DestinationCredentials.
type BackupDestination struct {
	SourceID string
	Name     string
	Provider string
	Endpoint string
	Region   string
	Bucket   string
	// HasCredentials is false when the source returned no keys or masked ones.
	HasCredentials bool
}

type dokployDestination struct {
	ID              string `json:"destinationId"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	AccessKey       string `json:"accessKey"`
	SecretAccessKey string `json:"secretAccessKey"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint"`
}

type dokployBackup struct {
	ID              string `json:"backupId"`
	Schedule        string `json:"schedule"`
	Enabled         bool   `json:"enabled"`
	DestinationID   string `json:"destinationId"`
	KeepLatestCount int    `json:"keepLatestCount"`
}

// credentialUsable rejects empty and masked values, so a masked key never reaches a create call.
func credentialUsable(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && strings.Trim(v, "*•x") != ""
}

func (s *DokploySource) readDestinations(ctx context.Context) ([]dokployDestination, error) {
	var out []dokployDestination
	if err := s.r.get(ctx, "/api/destination.all", nil, &out); err != nil {
		return nil, err
	}
	if len(out) > MaxPages {
		out = out[:MaxPages]
	}
	return out, nil
}

func (s *DokploySource) collectDestinations(ctx context.Context, d *Discovery) {
	list, err := s.readDestinations(ctx)
	if err != nil {
		d.Warnings = append(d.Warnings, "could not read backup destinations: "+err.Error())
		return
	}
	for _, x := range list {
		d.Destinations = append(d.Destinations, BackupDestination{SourceID: x.ID, Name: x.Name, Provider: x.Provider,
			Endpoint: strings.TrimSpace(x.Endpoint), Region: x.Region, Bucket: x.Bucket,
			HasCredentials: credentialUsable(x.AccessKey) && credentialUsable(x.SecretAccessKey)})
	}
}

// DestinationCredentials re-reads one destination's keys for an apply run. Callers pass
// them straight to the target's create call and keep them out of every report and log.
func (s *DokploySource) DestinationCredentials(ctx context.Context, sourceID string) (accessKey, secretKey string, err error) {
	list, err := s.readDestinations(ctx)
	if err != nil {
		return "", "", err
	}
	for _, x := range list {
		if x.ID == sourceID {
			if !credentialUsable(x.AccessKey) || !credentialUsable(x.SecretAccessKey) {
				return "", "", ErrNoCredentials
			}
			return x.AccessKey, x.SecretAccessKey, nil
		}
	}
	return "", "", ErrNoCredentials
}

// ErrNoCredentials means the source did not hand over usable keys for a destination.
var ErrNoCredentials = errors.New("the source returned no usable credentials for this destination")

func mapDokployBackups(list []dokployBackup) []BackupSchedule {
	out := make([]BackupSchedule, 0, len(list))
	for _, b := range list {
		out = append(out, BackupSchedule{Schedule: NormalizeSchedule(b.Schedule), Enabled: b.Enabled, Remote: b.DestinationID != "",
			SourceID: b.ID, DestinationID: b.DestinationID, KeepLatest: max(b.KeepLatestCount, 0)})
	}
	return out
}

// ValidateDestinationEndpoint applies the import network policy to an object-storage
// endpoint: https only, no credentials in the URL, and no blocked address behind the host.
func ValidateDestinationEndpoint(ctx context.Context, raw string, p NetworkPolicy) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("endpoint %q is not an absolute URL", raw)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("endpoint must use https, got %q", u.Scheme)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("endpoint must not carry credentials, a query or a fragment")
	}
	host := u.Hostname()
	if addr, err := netip.ParseAddr(host); err == nil {
		return p.Check(addr.Unmap())
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("endpoint host %q does not resolve: %w", host, err)
	}
	for _, a := range addrs {
		if err := p.Check(a.Unmap()); err != nil {
			return err
		}
	}
	return nil
}
