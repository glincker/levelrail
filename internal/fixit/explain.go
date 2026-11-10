package fixit

import "sort"

// Explanation is the fixed plain-language text for one doctor check. It is a
// lookup table, not generated text, so it works offline and never changes
// between runs.
type Explanation struct {
	ID    string `json:"id"`
	Plain string `json:"plain"`
	Why   string `json:"why"`
	// Next names the dashboard button or guided step first; a CLI command is only ever secondary.
	Next string `json:"next"`
	// FixID is the typed fix registered for the check, empty when there is none.
	FixID string `json:"fix_id,omitempty"`
	Kind  Kind   `json:"kind"`
}

// Explain returns the explanation for a check id.
func Explain(id string) (Explanation, bool) {
	e, ok := explainTable[id]
	return e, ok
}

// ExplainIDs lists every check id that has an explanation, sorted.
func ExplainIDs() []string {
	ids := make([]string, 0, len(explainTable))
	for id := range explainTable {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func ex(id, plain, why, next, fixID string, kind Kind) Explanation {
	return Explanation{ID: id, Plain: plain, Why: why, Next: next, FixID: fixID, Kind: kind}
}

const (
	nextFixButton = "Press Fix on this check in Doctor. "
	nextCLIFix    = "From a terminal, levelrail-cli doctor --fix does the same."
)

var explainTable = buildExplainTable()

func buildExplainTable() map[string]Explanation {
	list := []Explanation{
		// Runtime
		ex("docker", "The container engine is not answering.", "Nothing can build, deploy or restart until it answers.",
			"Start the engine on the host, then press Re-check on this check. From a terminal, systemctl start docker works on most Linux hosts.", "", KindManual),
		ex("docker_version", "The container engine is older than the oldest version this release is tested with.", "Old engines miss features builds and networks rely on.",
			"Upgrade the engine through your package manager, then press Re-check. The Docker docs list the supported install steps.", "", KindManual),
		ex("install_state", "This install is not in a normal released state.", "An unusual build or half finished upgrade can behave differently from what the docs describe.",
			"Open Settings, then Updates, and move to the latest release. From a terminal, levelrail-cli update does the same.", "", KindGuided),
		ex("container_runtime", "The container runtime is running with reduced support on this host.", "Some isolation or networking features may be missing.",
			"Open the check details to see what is missing, then follow the guided step. Nothing needs fixing if you accept the limits.", "", KindGuided),
		ex("docker_guard", "The engine socket guard is not enforcing yet.", "Without it a compromised container could talk to the engine directly.",
			"Open Settings, then Security, and switch the guard from audit to enforce once the audit log looks clean.", "", KindGuided),
		ex("docker_guard_high_risk", "A caller asked the engine for host-level power while the guard was only auditing.", "Enforce mode would have blocked it, and something may already hold that power.",
			"Open Settings, then Security, review the flagged rules in the issue list, then switch the guard to enforce.", "", KindGuided),
		ex("docker_privilege", "The engine is running with more privilege than needed.", "A bug in a container is easier to turn into host access.",
			"Open the check details for the setting to change, apply it on the host, then press Re-check.", "", KindGuided),
		ex("container_hardening", "Containers are not started with the hardened defaults.", "Dropped capabilities limit what a compromised container can do.",
			"Press Fix on this check to see the guided step. The policy lives in the service environment, so it is changed on the host.", "hardening.container.enforce", KindGuided),
		ex("network_pool", "The pool of private networks for apps is nearly full or holds unused networks.", "When it runs out, new apps fail to start.",
			nextFixButton+"It removes only networks no container uses. "+nextCLIFix, "docker.network_pool.prune", KindAuto),
		ex("docker_orphans", "There are containers, images or volumes no app owns.", "They use disk and can hold old data.",
			nextFixButton+"It shows what would go before anything is removed. "+nextCLIFix, "docker.orphans.prune", KindAuto),

		// Host
		ex("disk_space", "The disk that holds your data is nearly full.", "A full disk stops deploys, builds and databases.",
			nextFixButton+"It lists what can be reclaimed and asks first. "+nextCLIFix, "disk.reclaim", KindAuto),
		ex("inodes", "The data volume is running out of file entries, not space.", "Writes fail even when free space remains.",
			"Press Fix on this check for the guided step that finds the folder with the most small files. A terminal command is shown there as a second option.", "inodes.free", KindGuided),
		ex("docker_root_disk", "The disk holding container storage is low on space.", "Image pulls and builds fail when it fills up.",
			"Open Doctor, then Disk, and review the cleanup suggestions before you apply one.", "", KindGuided),
		ex("disk_hygiene", "Images, build cache or volumes are taking a lot of space.", "Left alone they slowly fill the disk.",
			"Open Doctor, then Disk, and review the suggested cleanup. Nothing is deleted until you confirm.", "", KindGuided),
		ex("data_dir_writable", "The control plane cannot write to its data folder.", "Settings, secrets and backups cannot be saved.",
			"Fix the folder owner or permissions on the host, then press Re-check. The check details show the path.", "", KindManual),
		ex("ram", "The host has little free memory.", "Builds and apps get killed when memory runs out.",
			"Stop apps you do not need or add memory or swap, then press Re-check.", "", KindManual),
		ex("build_memory", "The host has less memory than a typical web build needs.", "Builds may fail with out-of-memory errors.",
			"Add swap or memory, or build on a bigger node. Open Settings, then Builds, to pick the build node.", "", KindGuided),
		ex("cpu", "The host processor is busy most of the time.", "Everything on the host slows down.",
			"Open Resources to see which app uses the most, then lower its limit or move it to another node.", "", KindGuided),
		ex("disk_io_latency", "Writes to disk are slow.", "Every deploy and database on this host slows down, and freeing space does not help.",
			"Press Fix on this check for the guided step. A terminal command is shown there as a second option.", "disk.io.latency", KindGuided),
		ex("database", "The control plane database did not answer in time.", "Without it the dashboard and API cannot work.",
			"Check the disk and memory checks first, then press Re-check. If it keeps failing, restore from the latest backup.", "", KindManual),
		ex("idle_footprint", "The control plane uses more memory or processor than an idle install should.", "It leaves less room for your apps.",
			"Open Resources and look for a busy background job, then press Re-check.", "", KindGuided),

		// Network
		ex("port_http", "Port 80 is not available to the proxy.", "Certificates cannot be issued and plain http links fail.",
			"Open the check details to see which process holds the port, stop it, then press Re-check.", "", KindManual),
		ex("port_https", "Port 443 is not available to the proxy.", "Secure links to your apps will not load.",
			"Open the check details to see which process holds the port, stop it, then press Re-check.", "", KindManual),
		ex("firewall", "The host firewall blocks the web ports.", "People outside cannot reach your apps or get certificates.",
			nextFixButton+"It opens only ports 80 and 443. "+nextCLIFix, "host.firewall.open_web_ports", KindAuto),
		ex("network", "A network setting on this host is not as expected.", "Apps may not reach each other or the internet.",
			"Open the check details for the exact item, follow the guided step, then press Re-check.", "", KindGuided),
		ex("dns_domains", "One or more of your domains does not point at this server.", "Visitors reach the wrong place or get an error.",
			"Open the app, go to Domains and press Check DNS for the record to add at your DNS provider.", "", KindGuided),
		ex("dashboard_https", "The dashboard is served over plain http.", "Passwords and sessions cross the network unencrypted.",
			"Open Settings, then Domain, and add a domain for the dashboard, then press Re-check.", "", KindGuided),
		ex("dashboard_url_https", "The saved dashboard address does not use https.", "Links in emails and logins point at an insecure address.",
			nextFixButton+"It switches the saved address to https once a certificate works.", "settings.dashboard_url.https", KindAuto),
		ex("registry", "An image registry cannot be reached from this host.", "Builds and deploys that pull from it fail.",
			"Check the registry address under Settings, then Registries, and the host's outbound network. Press Re-check afterwards.", "", KindGuided),
		ex("registry_mirror", "GitHub, ghcr.io or Docker Hub cannot be reached from this host.", "Installs, upgrades, builds and node setup that download from them fail or stall.",
			"If your network restricts them, set a registry mirror under Settings and follow the install from mainland China page. Press Re-check afterwards.", "", KindGuided),
		ex("cross_node_ingress", "Traffic cannot get from the entry node to an app on another node.", "Visitors see a TLS or gateway error.",
			"Open Nodes, pick the node and press Test connection. The mesh checks explain what is blocked.", "", KindGuided),
		ex("ingress_edge", "The edge proxy is not routing as configured.", "Some domains may not serve the right app.",
			"Open Domains and press Re-sync on the affected domain, then press Re-check.", "", KindGuided),
		ex("ingress_survives", "The proxy may not come back by itself after a restart.", "A reboot could take your apps offline until someone notices.",
			"Open the check details for the missing restart setting and apply it, then press Re-check.", "", KindGuided),
		ex("exposure", "An app or database port is open to the internet.", "Anyone can try to connect to it.",
			"Open the app, go to Exposure and switch it to proxied or private unless the open port is on purpose.", "", KindGuided),
		ex("cloud", "The cloud account that backs this host reports a problem.", "Firewall rules or addresses may differ from what you expect.",
			"Open Settings, then Cloud, and reconnect the account or review the listed finding.", "", KindGuided),
		ex("certificates", "A certificate is expired or about to expire.", "Browsers show a warning and visitors leave.",
			nextFixButton+"It asks for a new certificate now.", "certs.renew", KindAuto),
		ex("certificate_budget", "You are close to the certificate issuer's rate limit.", "New domains may wait days for a certificate.",
			"Open Domains, then Certificates, to see the meter and remove domains you no longer use.", "", KindGuided),
		ex("reachability", "Some ports are not reachable from outside.", "Visitors cannot open your apps.",
			"Open Doctor, then Reachability, and follow the advice shown for each port.", "", KindGuided),
		ex("proxy_missing", "No proxy is installed to route web traffic.", "Your apps have no public address or certificate.",
			nextFixButton+"It installs the proxy. "+nextCLIFix, "proxy.install.traefik", KindAuto),
		ex("trusted_proxies", "The trusted proxy list does not match your setup.", "Client addresses in logs and rate limits may be wrong.",
			nextFixButton+"It saves the detected proxy addresses.", "settings.ingress.trusted_proxies", KindAuto),
		ex("webhook_public_url", "Webhooks have no public address to call back.", "Git pushes cannot trigger deploys.",
			nextFixButton+"It saves the dashboard address as the webhook address.", "settings.webhook_url.public", KindAuto),

		// Security
		ex("master_key_rotation", "The key that protects stored secrets is old.", "A leaked old key would expose every secret.",
			"Open Settings, then Security, and press Rotate master key.", "", KindGuided),
		ex("master_key_age", "The key that protects stored secrets has not been rotated for a long time.", "A leaked old key would expose every secret.",
			"Press Fix on this check for the guided step to rotate it.", "settings.master_key.age", KindGuided),
		ex("secret_binding", "A secret is not bound to the app that should use it.", "It may reach the wrong app or none.",
			"Open the app, go to Environment and bind the secret, then press Re-check.", "", KindGuided),
		ex("stale_secrets", "Some secret values have not changed in a long time.", "Old secrets widen the damage of a leak.",
			"Press Fix on this check for the guided step. Open each app's Environment tab and update the old values.", "secrets.stale.rotate", KindGuided),
		ex("notification_channel_secrets", "A notification channel is missing a secret it needs.", "Alerts for it will not be delivered.",
			"Open Settings, then Notifications, and re-enter the channel's secret.", "", KindGuided),
		ex("notify_reachable", "No notification channel or email provider is configured.", "You only learn about outages by looking at the dashboard.",
			"Open Settings, then Notifications, and add a channel, or add an email provider under Settings, then Email.", "", KindGuided),
		ex("secret_reveal_policy", "Showing secret values does not require a fresh confirmation.", "A stolen session could read secrets.",
			"Open Settings, then Security, and set secret reveal to require step-up.", "", KindGuided),
		ex("legacy_tokens", "Some API tokens use the old format.", "They cannot be limited or tracked like current tokens.",
			nextFixButton+"It converts them without changing their values. "+nextCLIFix, "tokens.legacy.migrate", KindAuto),
		ex("git_connections", "A Git connection lost its access.", "The next deploy from that repository will fail.",
			"Open Settings, then Git connections, and press Reconnect.", "", KindGuided),
		ex("token_hygiene", "Some API tokens never expire or are unused.", "A leaked token keeps working.",
			"Open Settings, then Tokens, and revoke or shorten the listed tokens.", "", KindGuided),
		ex("signature_pin", "Release signatures have not been pinned on this host.", "Upgrades accept releases with a checksum only.",
			"Open Settings, then Updates, and press Pin release signature.", "", KindGuided),
		ex("second_admin", "There is only one administrator.", "If that account is lost, nobody can sign in to manage the install.",
			"Press Fix on this check for the guided step to invite a second admin.", "settings.admin.second", KindGuided),
		ex("passkey_rp", "Passkeys are tied to an address you may change.", "Existing passkeys stop working if the address changes.",
			"Press Fix on this check for the guided step to set the passkey domain.", "settings.passkeys.rp_id", KindGuided),
		ex("email_sender", "No sender address is set for email.", "Invites and password resets cannot be sent.",
			"Press Fix on this check for the guided step to set the sender.", "settings.email.sender", KindGuided),

		// Backups
		ex("control_plane_backup", "The control plane has no recent backup.", "Losing the host would lose your settings and secrets.",
			nextFixButton+"It takes a backup now. "+nextCLIFix, "backup.control_plane.snapshot", KindAuto),
		ex("control_plane_dr", "The disaster recovery copy of the control plane is missing or old.", "A rebuilt host could not be restored.",
			nextFixButton+"It switches the schedule on when a destination and a recipient are saved. Otherwise open Settings, then Backups, and set an off-host destination.", "control_plane.dr.enable", KindAuto),
		ex("backup_freshness", "A database or volume backup is late.", "Recent data would be lost in a failure.",
			"Open Backups, find the late item and press Run now. Check its schedule if it keeps going late.", "", KindGuided),
		ex("backup_drill", "The latest restore test of a backup failed.", "A backup that cannot be restored is not a backup.",
			"Press Fix on this check for the guided step. Open Backups to see which restore test failed.", "backup.drill.failing", KindGuided),
		ex("schedule_never_runs", "A saved schedule has no date it can ever run on, so it never fires.", "Backups, deploys or tasks on that schedule silently never happen. Nothing was turned off for you.",
			"Open the app, database or task named in the check details and edit or delete the schedule.", "", KindManual),
		ex("backups_configured", "Backups are turned off.", "Nothing protects your data from loss.",
			"Press Fix on this check for the guided step to turn on backups.", "settings.backups.enabled", KindGuided),

		// Nodes and mesh
		ex("nodes", "A remote node is offline or runs an older agent.", "Apps on it may be unreachable or behave differently.",
			"Open Nodes, pick the node and press Reconnect, or Update agent if it is behind.", "", KindGuided),
		ex("mesh", "The private network between nodes is not ready.", "Nodes cannot talk to each other.",
			"Open Nodes, then Mesh, and follow the failing step shown there.", "", KindGuided),
		ex("wireguard", "The private network tool cannot run with the access it has.", "Nodes cannot join the mesh.",
			"Press Fix on this check for the guided step. It shows the host change as a second option.", "mesh.wireguard.privileges", KindGuided),
		ex("udp_port", "The mesh port is taken by another program.", "Nodes cannot open tunnels to this host.",
			"Press Fix on this check to see which program holds the port and the guided step to free it.", "mesh.udp_port.holder", KindGuided),
		ex("public_address", "Other nodes do not know this host's public address.", "They cannot connect back to it.",
			nextFixButton+"It offers the detected addresses to choose from.", "mesh.public_address.set", KindAuto),
		ex("container_log_rotation", "Some containers keep a log with no size limit.", "A chatty app can fill the disk over time.",
			"Open Settings, set the container log size and file count, then redeploy the apps so new containers use them.", "", KindGuided),
		ex("local_node", "This host is not registered as a node.", "The mesh cannot place apps on it.",
			"Open Nodes and press Add this host, then press Re-check.", "", KindGuided),
		ex("database_connections_high", "A database is close to its connection limit.", "New connections fail and apps show errors.",
			nextFixButton+"It turns on connection pooling for the database.", "database.connections.enable_pooler", KindAuto),

		// Hardware
		ex("gpus", "A graphics card is not usable by containers.", "GPU apps will not start.",
			"Open Nodes, pick the node and follow the GPU guided step to install the container toolkit.", "", KindGuided),
		ex("gpu_placement", "An app that needs a GPU is placed on a node without one.", "The app cannot start.",
			"Open the app, go to Placement and pick a node with a GPU.", "", KindGuided),
		ex("nas_client_tools", "The tools to mount network storage are missing on a node.", "Volumes on that storage cannot attach.",
			"Open Nodes, pick the node and follow the guided step to install the storage client, then press Re-check.", "", KindGuided),
	}
	m := make(map[string]Explanation, len(list))
	for _, e := range list {
		m[e.ID] = e
	}
	return m
}
