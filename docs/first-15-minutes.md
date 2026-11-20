---
title: "First 15 minutes"
sidebarTitle: "First 15 minutes"
description: "A short checklist for a new server: install Levelrail, open the setup link, turn on HTTPS, deploy an app, take a first backup and run the doctor check."
"og:url": "https://docs.levelrail.com/first-15-minutes"
---

## What should I do in the first 15 minutes on a new server?

Install, sign in, turn on HTTPS, deploy one app, take one backup and run the doctor. This page is a checklist, not a full guide: each step says what you should see and links to the page with the details. You need a fresh Linux server with root access and ports 80 and 443 open in the cloud firewall. For a slower walkthrough, use [Getting started](/getting-started).

## 1. Install

```bash
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/install.sh
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/checksums.txt
grep ' install.sh$' checksums.txt | sha256sum -c -  # macOS: shasum -a 256 -c -
sudo sh install.sh
```

You should see `Levelrail vX.Y.Z installed and running.` and a dashboard link with a one-time setup token. If it fails, see [Troubleshooting the first run](/getting-started#troubleshooting-the-first-run) or [Installing Levelrail](/installing). If the ports are not open yet, see [Cloud firewall rules](/cloud-firewall-rules).

## 2. Open the setup link

Port 8080 should stay closed to the internet, so tunnel it first. On your laptop:

```bash
ssh -L 8080:127.0.0.1:8080 <user>@<server-ip>
```

Open `http://127.0.0.1:8080/login#setup=<token>` (the link from the installer output with `127.0.0.1` as the host) and choose a password. You should land on the dashboard. If the link is lost or rejected, print the token again on the server:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data levelrail setup-token
```

If the installer chose another port, use that port in both places. Why 8080 stays closed: [Dashboard port 8080](/installing#dashboard-port-8080).

## 3. Turn on HTTPS

Press **Enable HTTPS** on the dashboard banner. No DNS record is needed. You get an address like `203-0-113-10.sslip.io` with a Let's Encrypt certificate. From a terminal:

```bash
levelrail-cli settings ingress https enable --wait 2m
levelrail-cli settings ingress https status
```

After this, open the dashboard on the https address, because sign-in over plain HTTP stops working. If it fails, see [Zero to HTTPS](/zero-to-https#5-when-it-does-not-work).

## 4. Deploy an app

Open **Get started** and deploy the sample app, pick a template under **Browse templates**, or paste a repository URL under **New app**. You should see the app turn healthy and get a live link. If the build fails, the dashboard shows a diagnosis and a link to the logs. More options: [Getting started](/getting-started#3-deploy-a-one-click-template) and [Templates](/templates-and-registry).

## 5. Take your first backup

Back up `/var/lib/levelrail-data/master.key` first. Without it, stored secrets cannot be read after a restore. Then take a snapshot of the control plane database:

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
- [Troubleshooting](/troubleshooting) and the [FAQ](/faq).
