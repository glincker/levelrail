---
title: "First 15 minutes"
sidebarTitle: "First 15 minutes"
description: "A checklist for a new server: install, open the setup link, turn on HTTPS, deploy an app and take your first backup. Each step links to the full page."
"og:url": "https://docs.levelrail.com/first-15-minutes"
---

This is a checklist, not a full guide. Do the steps in order. Each one says what you should see and where to look if it fails. You need a fresh Linux server with root access and ports 80 and 443 open in the cloud firewall.

## 1. Install

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

You should see a dashboard link with a one-time setup token. If it fails, see [Installing Levelrail](/installing). If the ports are not open yet, see [Cloud firewall rules](/cloud-firewall-rules).

## 2. Open the setup link

Port 8080 should stay closed to the internet, so tunnel it first. On your laptop:

```bash
ssh -L 8080:127.0.0.1:8080 <user>@<server-ip>
```

Open the link from the installer output with `localhost` as the host, and choose a password. You should land on the first-run screen. If the link is lost or rejected, see [Getting started](/getting-started#sign-in-and-deploy-your-first-app) or print the token again:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data levelrail setup-token
```

Why 8080 stays closed: [Dashboard port 8080](/installing#dashboard-port-8080).

## 3. Turn on HTTPS

On the first-run screen, press **Enable HTTPS**. No DNS record is needed. You get an address like `203-0-113-10.sslip.io` with a Let's Encrypt certificate. From a terminal:

```bash
levelrail-cli settings ingress https enable --wait 2m
levelrail-cli settings ingress https status
```

After this, open the dashboard on the https address and sign-in over plain HTTP stops working. If it fails, see [Zero to HTTPS](/zero-to-https#5-when-it-does-not-work).

## 4. Deploy an app

On the same screen, deploy a sample app or pick a template. You can also paste a repository URL under **New app**. The screen waits until the app is healthy and ends on its live link. If the build fails, the screen shows a diagnosis and a link to the logs. More options: [Getting started](/getting-started#deploy-your-first-app) and [Templates](/templates-and-registry).

## 5. Take your first backup

Back up `master.key` first. Without it, stored secrets cannot be read after a restore. Then take a snapshot of the control plane database:

```bash
levelrail-cli control-plane-backups create
levelrail-cli control-plane-backups list
```

You should see one snapshot in the list. It lives on the server's disk, so it does not protect you if the server is lost. The rest of the path, including an off-server target, is in [First backup in 5 minutes](/backups-and-storage#first-backup-in-5-minutes).

## 6. Check the server

```bash
levelrail-cli doctor
```

Read any `fail` rows and fix them. A `warn` on external reachability is a hint, not proof: test from another machine. See [Doctor](/doctor).

## What next

- [Zero to HTTPS](/zero-to-https): move from the test address to your own domain.
- [Go live checklist](/go-live): point a domain at an app.
- [Domains and ingress](/domains-and-ingress): how names and certificates work.
- [Backups and storage](/backups-and-storage): schedules and restores.

## Related

- [Installing Levelrail](/installing)
- [Getting started](/getting-started)
- [Cloud firewall rules](/cloud-firewall-rules)
- [Troubleshooting](/troubleshooting)
