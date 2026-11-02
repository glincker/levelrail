package appexport

import (
	"fmt"
	"sort"
	"strings"
)

// dbDump returns dump and restore commands for a known database image.
func dbDump(image string) (dump, restore string, ok bool) {
	name := image
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[:i]
	}
	switch name {
	case "postgres", "postgis", "timescaledb", "pgvector":
		return `docker compose exec -T %s pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > %s.sql`,
			`docker compose exec -T %s psql -U "$POSTGRES_USER" "$POSTGRES_DB" < %s.sql`, true
	case "mysql", "mariadb":
		return `docker compose exec -T %s sh -c 'mysqldump -u root -p"$MYSQL_ROOT_PASSWORD" --all-databases' > %s.sql`,
			`docker compose exec -T %s sh -c 'mysql -u root -p"$MYSQL_ROOT_PASSWORD"' < %s.sql`, true
	case "mongo":
		return `docker compose exec -T %s mongodump --archive > %s.archive`,
			`docker compose exec -T %s mongorestore --archive < %s.archive`, true
	}
	return "", "", false
}

func renderReadme(app string, svcs []Service) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s on plain Docker Compose\n\n", app)
	b.WriteString("Exported from Levelrail. Nothing here needs Levelrail to run.\n\n")
	b.WriteString("## Start\n\n1. `cp .env.example .env`\n2. Replace every `CHANGE_ME` in `.env`. Secrets are never exported; re-enter them or restore them from your own backup.\n3. `docker compose up -d`\n\n")
	b.WriteString("## Data\n\nNamed volumes start empty. To restore a volume from a tar archive of its contents (for example one made with a Levelrail backup download):\n\n")
	b.WriteString("```\ndocker compose create\n")
	for _, v := range volumeNames(svcs) {
		fmt.Fprintf(&b, "docker run --rm -v %s_%s:/data -v \"$PWD\":/backup alpine tar xzf /backup/%s.tar.gz -C /data\n", projectName(app), v, v)
	}
	b.WriteString("```\n\n")
	var vols []string
	for _, s := range svcs {
		for _, v := range s.Volumes {
			vols = append(vols, fmt.Sprintf("%s (%s, mounted at %s)", volumeKey(v.Name), s.Key, v.ContainerPath))
		}
	}
	sort.Strings(vols)
	for _, v := range vols {
		fmt.Fprintf(&b, "- volume %s\n", v)
	}
	if len(vols) > 0 {
		b.WriteString("\n")
	}
	var dbs []Service
	for _, s := range svcs {
		if _, _, ok := dbDump(s.Image); ok {
			dbs = append(dbs, s)
		}
	}
	if len(dbs) > 0 {
		b.WriteString("## Databases\n\nThe database services keep the same engine and version tag as before. Dump on the old host, restore on the new one:\n\n```\n")
		for _, s := range dbs {
			dump, restore, _ := dbDump(s.Image)
			fmt.Fprintf(&b, "# %s (%s)\n"+dump+"\n"+restore+"\n", s.Key, s.Image, s.Key, s.Key, s.Key, s.Key)
		}
		b.WriteString("```\n\n")
	}
	b.WriteString("## Domains and TLS\n\n")
	hasDomains := false
	for _, s := range svcs {
		if len(s.Domains) > 0 {
			hasDomains = true
			d := append([]string(nil), s.Domains...)
			sort.Strings(d)
			fmt.Fprintf(&b, "- %s: %s (port %d)\n", s.Key, strings.Join(d, ", "), s.Port)
		}
	}
	if hasDomains {
		b.WriteString("\nCertificates are not exported. Point these names at your host and put any reverse proxy with automatic TLS in front of the published port. The `x-domains` field in the compose file is ignored by Docker.\n\n")
	} else {
		b.WriteString("No domains were configured.\n\n")
	}
	var notes []string
	for _, s := range svcs {
		if len(s.BindMounts) > 0 {
			notes = append(notes, fmt.Sprintf("- %s uses host bind mounts; make sure those paths exist on the new host.", s.Key))
		}
		if len(s.FilePaths) > 0 {
			p := append([]string(nil), s.FilePaths...)
			sort.Strings(p)
			notes = append(notes, fmt.Sprintf("- %s had managed config files (%s); their contents are not exported, recreate them as bind mounts.", s.Key, strings.Join(p, ", ")))
		}
	}
	if len(notes) > 0 {
		b.WriteString("## Check\n\n" + strings.Join(notes, "\n") + "\n\n")
	}
	b.WriteString("## Not exported\n\nSecret values, backup data, TLS certificates, and Levelrail features such as previews, auto-sleep and deploy hooks. Private registry logins must be redone with `docker login`.\n")
	return b.String()
}

func volumeNames(svcs []Service) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range svcs {
		for _, v := range s.Volumes {
			if k := volumeKey(v.Name); !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}
