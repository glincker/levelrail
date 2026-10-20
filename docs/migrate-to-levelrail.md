---
title: "Migrate to Levelrail"
sidebarTitle: "Migration overview"
description: Move from Coolify, Dokploy, CapRover, Dokku or a compose file with the old server kept running, an honest downtime table and a rollback path.
"og:url": "https://docs.levelrail.com/migrate-to-levelrail"
---

This is the starting point for moving a running setup onto Levelrail. It covers what stays online, what does not, and the order to do things in. Each platform has its own page with exact commands.

The short version: the old server keeps serving while you stage, copy and test the new copy. Stateless apps can move with no outage you would notice. Databases cannot: they need a short write freeze, and its length depends on how much data you have. Nothing here is live replication.

## Pick your starting point

| You run | Go to | What the importer reads |
| --- | --- | --- |
| Coolify | [Migrate from Coolify](/migrate-from-coolify) | Its HTTP API (read-only), or a `docker inspect` snapshot |
| Dokploy | [Migrate from Dokploy](/migrate-from-dokploy) | Its HTTP API (read-only) |
| CapRover | [Migrate from CapRover](/migrate-from-caprover) | Its HTTP API (read-only) |
| Dokku | [Migrate from Dokku](/migrate-from-dokku) | A pasted `dokku config:export` dump. Live Dokku over SSH is not supported |
| A `docker-compose.yml` | [Migrate from Docker Compose](/migrate-from-docker-compose) | The compose file you paste or pass |
| Heroku-style or plain git apps | [Importing apps](/importing-apps) | Nothing to import: point Levelrail at the repo, image or Dockerfile |
| Vercel or Cloud Run | [From Vercel](/migrating-from-vercel), [from Cloud Run](/migrating-from-cloud-run) | Separate guides |

## The six steps

<Steps>
<Step title="Inventory">

List what runs on the old server: apps, databases, volumes, domains, cron jobs, and the secrets each app needs. Note which apps write to local disk and which databases take writes all day. That tells you which apps are the easy ones.

</Step>
<Step title="Install and connect">

[Install Levelrail](/installing) on the new server (or beside the old one, see [Behind an existing proxy](/behind-an-existing-proxy)). It needs its own public address, ports 80 and 443 free, and a network route to the old server if you want it to copy databases directly.

</Step>
<Step title="Plan with a dry run">

Read the old platform without changing anything. You get one verdict per item and a reason for each. Fix the blockers it names (a name collision, a missing port) and run it again. The old platform only ever receives read requests.

</Step>
<Step title="Stage and verify">

Create the apps here stopped and not routed, copy images, data and volumes, then start each app and let it pass its own health check. Test it directly, for example with `curl --resolve`, while the old server still serves all public traffic.

</Step>
<Step title="Cutover, one domain at a time">

Lower DNS TTLs a day ahead. Run the cutover check, freeze writes for apps with databases, do the final data copy, then switch DNS and verify. Do the least important domain first.

</Step>
<Step title="Retire the old server">

Keep it running, but idle, for at least one full traffic cycle (a week for a weekly job). Stop it only after you have checked backups, cron jobs and logs on the new side.

</Step>
</Steps>

## What stays online during the move

The old server is never stopped by Levelrail. Staging, verifying and dry runs all happen next to it. The only moments users can notice are the final data copy and the DNS switch.

| What you move | Expected downtime | Why | How to keep it small |
| --- | --- | --- | --- |
| Stateless app (an image or a git build, no volumes) | None you should notice, if the old copy stays up and the TTL was lowered | Both copies serve the same thing. Clients with a cached address keep reaching the old one until the TTL ends. | Lower the TTL to 300 seconds or less a day ahead. Switch a low traffic domain first and check its certificate. |
| App with volumes | The time of the last file sync, with the app paused on the old side | Files written after the last sync are not carried. The bulk copy can run live. | Run the sync once live, then again right before the switch with the old app stopped. Later runs only send the differences. |
| Database (PostgreSQL, MySQL, MariaDB, MongoDB, Redis) | A write freeze from the start of the final copy until the new app is live | The copy is a full dump and restore. There is no incremental sync and no replication. | Time the first copy: the final one takes about as long. Pause the old app, copy, start the new app, switch DNS. |
| Not possible today | Zero downtime for a database. Writes accepted by the old database after the final copy. Live replication between old and new. | The importer only reads and copies. It does not stream changes. | Choose a quiet hour and tell users. |

A first HTTPS request to a domain that was just pointed here can be slow or show a certificate warning. Certificates are issued on first use after DNS points here (HTTP-01 needs port 80 reachable), and until then the platform's own certificate answers. Allow a minute, and check the first domain before you switch the rest. See [Domains and ingress](/domains-and-ingress).

## Pre-flight checklist

- [ ] **Backups.** A fresh backup of every database and volume from the old server, stored somewhere that is not that server. Test one restore.
- [ ] **DNS TTL.** Lower the TTL of every domain you will move to 300 seconds (60 if your provider allows) at least one old-TTL ahead of the switch. The cutover check reads it and tells you to wait if it is above 300.
- [ ] **Access to your DNS provider.** Either connect Cloudflare or Route 53 in Levelrail so the switch can write the record and store the old value, or be ready to edit the record by hand.
- [ ] **Disk.** The new node needs room for every image, volume and database copy. The hub preflight checks free disk for databases with a margin.
- [ ] **Ports.** 80 and 443 reachable from the internet on the new server for certificates. If they are taken by another proxy, use coexist mode.
- [ ] **Versions.** Create each target database with the same or a newer major version than the source. `pg_dump` refuses to read a newer server.
- [ ] **Source credentials.** A read-only token for the old platform (Coolify needs `read:sensitive` to return real secret values), and a database user that can read everything you want copied.
- [ ] **Secrets.** Know which variables came back empty. They are flagged in the plan and you set them by hand.
- [ ] **A maintenance page or read-only mode** for apps that write to a database, so the freeze is deliberate.

## Cutover checklist

1. Plan shows every item you are moving as ready, or with a note you have read.
2. Each app is healthy here (`import apps --verify` or its own readiness check).
3. Each database has a verified copy with matching row counts, and you have timed it.
4. Each volume has matching file counts and sizes on both hosts.
5. TTLs are at or below 300 seconds and the old TTL has expired.
6. `levelrail-cli migrate cutover` says `go` for the domain.
7. Old app paused (writes frozen), final database copy and volume sync done and verified.
8. DNS switched, then `levelrail-cli migrate cutover --verify` or the guided cutover passes.
9. Watch logs and error rate on the new side for a while before the next domain.

## Rollback

Until you stop the old server, rollback is cheap.

- **Before the DNS switch:** delete what the import created, or run `levelrail-cli import apps --session ID --rollback` for a guided import. The source is untouched, so nothing else needs undoing. Volumes created here are kept on disk.
- **After the DNS switch:** point the record back at the old server. With a low TTL that takes minutes. The guided cutover does this for you (`levelrail-cli import cutover rollback`), restoring the stored previous value of a record it wrote through Cloudflare or Route 53. Rollback does not restore a Cloudflare proxy (orange cloud) setting, so check it.
- **Data written here after the switch** is not copied back. If users already wrote to the new database, you decide which side is the truth. This is the main reason to keep the freeze window short and to roll back early or not at all.

## Migration status

Maturity here uses the labels on [Feature status](/feature-status). The migration importer is not yet listed there. By that page's definition it is **beta**: it works and is tested, but has no end-to-end suite and almost no record on real infrastructure.

| Source | Maturity | What backs it |
| --- | --- | --- |
| Coolify | beta | Unit tests against a fake Coolify API and fixtures. Database copy rehearsed live on two droplets for PostgreSQL, MySQL and MongoDB. Guided cutover has a live Docker test. |
| Dokploy | beta | Unit tests against fixtures. No live Dokploy run recorded. |
| CapRover | beta | Unit tests against fixtures. No live CapRover run recorded. |
| Dokku (env dump) | beta | Unit tests. Variables only, no live Dokku run recorded. |
| Docker Compose file | beta | Unit tests. No live rehearsal recorded. |
| Docker snapshot (`docker inspect`) | beta | Unit tests. Works for any host that runs plain containers. |

If something fails, the plan says why and what to do by hand. Tell us what broke on [GitHub](https://github.com/glincker/levelrail/issues).

## FAQ

<AccordionGroup>
  <Accordion title="Does the old server need to stay up?">
    Yes, through the whole move. It serves traffic while you stage and test, it is the rollback target, and the data copy reads from it. Levelrail never stops or changes it.
  </Accordion>
  <Accordion title="Can I migrate one app at a time?">
    Yes, and you should. Use `--only NAME` to stage a subset, switch one domain at a time, and keep the rest where they are. Apps that share a database should move in the same window.
  </Accordion>
  <Accordion title="What happens to my environment secrets?">
    Secret-looking variables go through the encrypted secrets store here, never plain env. The source token is held in memory for the request and never stored, logged or returned. A variable the source hid from your token comes back empty and is skipped, with a note.
  </Accordion>
  <Accordion title="What about volumes?">
    The control plane does not copy volume contents, because that needs root on the old host. It prints one `rsync` command per volume, you run it, and a checklist step compares file counts and bytes on both sides.
  </Accordion>
  <Accordion title="How long does it take?">
    Planning and staging take minutes. Image moves and copies scale with size. The only number that matters for users is the final database copy, which you can measure with a first copy. Plan a day for the DNS TTL to settle, and a week before retiring the old server.
  </Accordion>
  <Accordion title="Can I undo it?">
    Before the old server is stopped, yes: see Rollback above. After you stop and delete it, only your backups remain.
  </Accordion>
  <Accordion title="Does it run commands on my old server?">
    No. The only exception is the optional image move, which opens an SSH connection and runs `docker save` on images from the plan, nothing else.
  </Accordion>
</AccordionGroup>

## See also

- [Migrate from Coolify](/migrate-from-coolify), [Dokploy](/migrate-from-dokploy), [CapRover](/migrate-from-caprover), [Dokku](/migrate-from-dokku), [Docker Compose](/migrate-from-docker-compose)
- [Move to the same server](/migrate-same-server) and [import over SSH](/migrate-over-ssh)
- [Managing databases](/managing-databases) and [Backups and storage](/backups-and-storage)
- [Domains and ingress](/domains-and-ingress)
- [Comparison](/comparison)
