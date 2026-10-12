---
title: "What you need before you start"
icon: "/assets/icons/clipboard-text.svg"
sidebarTitle: "What you need"
description: "A plain-language checklist for installing Levelrail: a Linux server, a few ports, and optionally a domain. No Kubernetes, no prior DevOps experience."
---

Levelrail runs on a Linux server that you own or rent. You install it with one command, then use a web dashboard to deploy apps. This page lists everything you need up front.

## The short version

- A Linux server: a small virtual private server (VPS) from any hosting provider, or a spare machine at home.
- About 10 minutes.
- Optionally, a domain name. You can start without one and add it later.

## 1. A server

A server is just a computer that stays on and is reachable from the internet. Most people rent a small virtual one (a VPS) from a hosting provider such as Hetzner, DigitalOcean or Linode. Any provider that gives you root access to a Linux machine works.

| Item | What to pick |
| --- | --- |
| Operating system | Ubuntu 24.04 or Debian 12 (both are tested). Other Linux distributions with systemd should work. |
| Size to start | 1 CPU core, 1 GB of memory and 10 GB of disk will boot Levelrail. 2 CPU cores, 2 GB of memory and 20 GB of disk is more comfortable once you build and run apps. |
| Access | `root`, or a user that can run `sudo`, over SSH. |
| Network | Ports `80` and `443` reachable from the internet. Port 80 is how free HTTPS certificates are issued. |

You do not need to install Docker yourself. The installer does it if it is missing.

<Note>
The sizes above are a starting point, not a benchmarked minimum. Start small and resize the server later if you need to.
</Note>

## 2. A domain name (optional)

A domain such as `apps.example.com` gives your apps real HTTPS addresses. It is not required to get started: Levelrail can give you a working address without any DNS setup, and you can attach your own domain later from [Domains and ingress](/domains-and-ingress).

If you do have a domain, you will point an `A` record at your server's IP address. The dashboard shows you exactly what to enter.

## 3. A way to reach the server

You will connect over SSH from your own computer. On macOS and Linux that is the `ssh` command in a terminal. On Windows, use PowerShell or Windows Terminal. Your hosting provider shows the command, usually `ssh root@YOUR_SERVER_IP`.

## What happens next

<Steps>
  <Step title="Install">
    Run the install command on the server. See [Install on a server](/installing).
  </Step>
  <Step title="Sign in">
    Open the dashboard in your browser and use the one-time setup token the installer prints.
  </Step>
  <Step title="Deploy your first app">
    Follow the [quickstart](/getting-started) to run something real in about ten minutes.
  </Step>
</Steps>

## Good to know

- Levelrail is one program plus Docker. There is no cluster to build or learn.
- Your apps and data stay on your server. Nothing is sent to a hosted service.
- If a step fails, the [troubleshooting guide](/troubleshooting) covers the common causes.
