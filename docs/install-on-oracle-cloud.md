---
title: "Install on Oracle Cloud Always Free (Arm)"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Oracle Cloud Always Free"
description: "Create an Always Free Arm VM on Oracle Cloud, open both firewall layers, install Levelrail, and what to know about capacity and idle reclamation."
"og:url": "https://docs.levelrail.com/install-on-oracle-cloud"
---

This page covers running Levelrail on an Oracle Cloud Always Free Arm VM. The install itself is the same `install.sh` as everywhere else, see [Installing Levelrail](/installing). What is specific to Oracle is the shape, the two firewalls, capacity errors and the idle rule.

Oracle changes its free tier limits and rules. Treat every number here as a pointer to Oracle's page, not as a promise. The source for the limits and the idle rule is [Always Free Resources](https://docs.oracle.com/en-us/iaas/Content/FreeTier/resourceref.htm). Check it and your own console before you plan around a figure.

## What the free Arm shape gives you

Oracle's page says an Always Free tenancy gets the equivalent of 2 OCPUs and 12 GB of memory on the `VM.Standard.A1.Flex` shape, which you can split across instances. It also says boot volumes and block volumes share a 200 GB total, and that outbound data is 10 TB per month (all per the page linked above). Older guides quote 4 OCPUs and 24 GB. Do not assume that: the console shows what your tenancy actually has.

Levelrail's own release binaries and container images are built for `arm64` as well as `amd64`, so the control plane runs on this shape. The apps you deploy are a separate question, see [Arm and app images](#arm-and-app-images).

## 1. Create the instance

1. In the Oracle console, create a compute instance. Pick the `VM.Standard.A1.Flex` shape (Ampere).
2. Pick a Canonical Ubuntu 24.04 image for `aarch64`. Avoid the Minimal variant.
3. Set the OCPU and memory sliders to what your console allows. Levelrail itself needs little (see [Installing Levelrail](/installing) for minimums), the rest is for your apps.
4. Make the boot volume larger than the default if the console lets you. Docker images, build cache and volumes live on the boot volume by default, and the free storage total is shared with any block volumes.
5. Add your SSH public key. The login user on Ubuntu images is `ubuntu`.
6. Create the instance and note its public IP.

### If you get "out of host capacity"

Oracle's page describes this error as a temporary lack of Always Free shapes in your home region. Its suggestions are to try a different availability domain, wait and try again, or upgrade the account to Pay As You Go. Your home region is chosen at signup, so choose it with care. Levelrail does not ship a retry tool.

### Keep the public IP

A public IP that is not reserved can change when the instance is stopped and started. If you point DNS or give players an address, reserve the IP in the console. Check Oracle's pricing page for how reserved IPs are billed on your account type.

## 2. Open the cloud firewall (layer one)

Traffic reaches the VM only if the VCN allows it. In the console open the instance's subnet, then its security list (or the network security group attached to the instance) and add ingress rules.

| Source CIDR | Protocol | Destination port | Used for |
| --- | --- | --- | --- |
| `0.0.0.0/0` | TCP | 80 | Certificate issuance and HTTP |
| `0.0.0.0/0` | TCP | 443 | Dashboard and apps over HTTPS |
| `0.0.0.0/0` | UDP | 443 | HTTP/3 (optional) |
| `0.0.0.0/0` | TCP | 22 | SSH (usually present by default) |

Add rules for any app port you publish directly, for example TCP 25565 for a Minecraft Java server, see [Game servers](/game-servers). Add `::/0` rules only if your VCN has IPv6.

## 3. Open the host firewall (layer two)

Opening the VCN is not enough. The images also carry a host firewall, and it is a separate switch. Which one depends on the image:

- Ubuntu images from Oracle commonly ship an iptables ruleset that allows SSH and rejects the rest at the end of the `INPUT` chain. `ufw` is usually not installed.
- Oracle Linux images use firewalld.

Look before you change anything:

```bash
# check with sudo iptables -S INPUT first
sudo iptables -S INPUT
which ufw firewalld
```

The installer warns when it finds an active firewalld. For `iptables`, the commands below insert allow rules ahead of the final reject. Check the rule order in the output of the command above before you run them, because the right position depends on your ruleset:

```bash
# check with sudo iptables -S INPUT first
sudo iptables -L INPUT --line-numbers
sudo iptables -I INPUT 5 -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -p tcp --dport 443 -j ACCEPT
sudo iptables -I INPUT 7 -p udp --dport 443 -j ACCEPT
```

The rule position (`5`, `6`, `7`) is an example. Use a position that sits before the `REJECT` line in your own listing. These rules are lost on reboot unless you save them. On Ubuntu, if the `netfilter-persistent` package is installed:

```bash
sudo netfilter-persistent save
```

The go-live checklist in the dashboard can read the host firewall and open ports 80 and 443 for you when the control plane runs as root, and it shows Oracle Cloud steps for the VCN layer. It never edits existing rules. See [Go live](/go-live) and [Host firewall](/host-firewall).

Ports that Docker publishes for an app normally do not pass through the `INPUT` chain, so they may be reachable with only the VCN rule. Test from outside the network rather than assuming, and test UDP separately, see [UDP published ports](/udp-ports).

## 4. Install Levelrail

SSH in and run the installer:

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

Then follow [Installing Levelrail](/installing) and [Go live](/go-live). Run the readiness check from the dashboard or with `levelrail-cli readiness` to see which ports answer from outside. If 80 or 443 show as blocked while the host firewall looks right, recheck the VCN rules from step 2.

## Arm and app images

An app runs on this VM only if its container image has a `linux/arm64` build. Many do, some do not. The template catalog does not guarantee every template works on Arm. Before you install a template, check the image's published platforms (for example with `docker buildx imagetools inspect <image:tag>`), and see [Template catalog](/template-catalog) and [Templates status](/templates-status), which lists templates known to fail on Arm. If an image has no arm64 build you will see a pull error or an `exec format error`.

## Idle reclamation

Oracle's page says idle Always Free compute instances may be reclaimed. It defines idle as a 7-day period in which the 95th percentile CPU use is under 20 percent, network use is under 20 percent, and memory use is under 20 percent (the memory test applies to A1 shapes only). That is Oracle's rule as published at the link above. It can change, and the page does not describe in detail what reclaiming means for your instance.

What this means for you:

- A lightly used personal server can meet those conditions. Levelrail does not change that.
- Oracle's page says that after you upgrade to Pay As You Go it does not charge for Always Free resources, only for usage above the free limits. Whether to upgrade is your call. Read Oracle's terms and set a budget alert if you do. This page does not promise any particular outcome.
- Levelrail does not include a keep-alive or load generator, and this guide does not suggest one.
- Keep backups off the VM, see [Backup targets and volume backups](/backups-and-storage), so a lost instance does not mean lost data.

## Related

- [Install on a cloud VM](/install-on-cloud)
- [Installing Levelrail](/installing)
- [Go live](/go-live)
- [Host firewall](/host-firewall)
- [Game servers](/game-servers)
