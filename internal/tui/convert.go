package tui

import (
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Row mappers copy only display fields: env values, secrets and tokens
// are never read into the UI model.

func appRows(in []apiclient.AppListEntry) []AppRow {
	out := make([]AppRow, 0, len(in))
	for _, a := range in {
		out = append(out, AppRow{
			Name:     a.Name,
			Status:   a.Status.Label,
			Tone:     variantTone(a.Status.Variant),
			Image:    a.Image,
			Strategy: a.Strategy,
			Node:     a.NodeID,
			Replicas: a.Replicas,
			Domains:  a.Domains,
		})
	}
	return out
}

func variantTone(v string) Tone {
	switch v {
	case "success":
		return ToneSuccess
	case "destructive", "danger":
		return ToneDanger
	case "warning":
		return ToneWarning
	case "info":
		return ToneInfo
	default:
		return ToneNeutral
	}
}

func deployRows(in []apiclient.DeploymentResource) []DeployRow {
	out := make([]DeployRow, 0, len(in))
	for _, d := range in {
		row := DeployRow{
			ID: d.ID, App: d.App, Status: d.Status, Trigger: d.Trigger,
			Env: d.Environment, Commit: d.CommitSHA, Branch: d.Branch,
			StartedAt: d.StartedAt, IsLive: d.IsLive,
		}
		if d.DurationMS != nil {
			row.DurationMS = *d.DurationMS
		}
		out = append(out, row)
	}
	return out
}

func attentionRows(in []apiclient.AttentionFeedItem) []AttentionRow {
	out := make([]AttentionRow, 0, len(in))
	for _, a := range in {
		out = append(out, AttentionRow{Severity: a.Severity, Kind: a.Kind, Subject: a.Subject, Title: a.Title, Detail: a.Detail})
	}
	return out
}

func dbRows(in []apiclient.DatabaseResource) []DBRow {
	out := make([]DBRow, 0, len(in))
	for _, d := range in {
		out = append(out, DBRow{
			Name: d.Name, Engine: d.Engine, Version: d.Version, Node: d.NodeID,
			Schedule: d.BackupSchedule, Retain: d.BackupRetain,
			Public: d.PubliclyAccessible, Stopped: d.Suspended,
		})
	}
	return out
}

func nodeRows(in []apiclient.NodeResource) []NodeRow {
	out := make([]NodeRow, 0, len(in))
	for _, n := range in {
		row := NodeRow{Name: n.Name, Address: n.Address, Status: n.Status, Schedulable: n.Schedulable, Local: n.IsLocal}
		if n.LastSeenAt != nil {
			row.LastSeen = *n.LastSeenAt
		}
		out = append(out, row)
	}
	return out
}

func domainRows(in []apiclient.DomainResource) []DomainRow {
	out := make([]DomainRow, 0, len(in))
	for _, d := range in {
		out = append(out, DomainRow{Domain: d.Domain, App: d.ServiceName, TLS: d.TLSOwner, Kind: d.Kind})
	}
	return out
}

func backupRows(in []apiclient.BackupHistoryResource) []BackupRow {
	out := make([]BackupRow, 0, len(in))
	for _, b := range in {
		name := b.DatabaseName
		if name == "" {
			name = b.ServiceName
			if b.VolumeName != "" {
				name += "/" + b.VolumeName
			}
		}
		started, _ := time.Parse(time.RFC3339Nano, b.StartedAt)
		out = append(out, BackupRow{Resource: name, Kind: b.ResourceKind, Status: b.Status, SizeBytes: b.SizeBytes, StartedAt: started})
	}
	return out
}

func meshInfo(m apiclient.MeshStatusResource) MeshInfo {
	info := MeshInfo{Enabled: m.Enabled, Peers: len(m.Peers)}
	for _, p := range m.Peers {
		if p.Healthy {
			info.Healthy++
		}
	}
	return info
}

func deployTone(status string) Tone {
	switch strings.ToLower(status) {
	case "succeeded", "success", "live":
		return ToneSuccess
	case "failed", "error":
		return ToneDanger
	case "running", "queued":
		return ToneInfo
	case "held", "pending":
		return ToneWarning
	default:
		return ToneNeutral
	}
}

func nodeTone(status string) Tone {
	switch strings.ToLower(status) {
	case "online":
		return ToneSuccess
	case "offline":
		return ToneDanger
	case "pending", "cordoned":
		return ToneWarning
	default:
		return ToneNeutral
	}
}

func severityTone(sev string) Tone {
	switch strings.ToLower(sev) {
	case "critical":
		return ToneDanger
	case "warning":
		return ToneWarning
	case "info":
		return ToneInfo
	default:
		return ToneNeutral
	}
}
