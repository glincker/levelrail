---
title: "Install on Oracle Cloud Always Free (Arm)"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Oracle Cloud Always Free"
description: "Create an Always Free Arm VM on Oracle Cloud, open both firewall layers, install Levelrail, and what to know about capacity and idle reclamation."
"og:url": "https://docs.levelrail.com/install-on-oracle-cloud"
---

This page covers running Levelrail on an Oracle Cloud Always Free Arm VM. The install itself is the same `install.sh` as everywhere else, see [Installing Levelrail](/installing). What is specific to Oracle is the shape, the two firewalls, capacity errors and the idle rule.

Oracle changes its free tier limits and rules. Treat every number here as a pointer to Oracle's page, not as a promise. The source for the limits and the idle rule is [Always Free Resources](https://docs.oracle.com/en-us/iaas/Content/FreeTier/resourceref.htm). Check it and your own console before you plan around a figure.

<Note>
The doctor cloud checks (`cloud_provider`, `cloud_firewall_layers`, `host_firewall_iptables`, `published_port_reachability`, `image_arch`, `idle_reclaim_risk`) and the `PlatformMismatch` refusal described below are available from the release that includes them. A release published before that does not have them: on the latest published release at the time of the 2026-11-01 test run, `levelrail-cli doctor` showed none of the cloud checks. Check `levelrail-cli version` and the [changelog](/changelog) before you rely on them.
</Note>

## Set the guards first

Do this before you create anything. Oracle's API does not mark a create as free or billable, so a mistake only shows up on the bill. Two guards stop that.

Which account you have decides how much you need them. The test run for this page used a Free Trial account, not a strictly Always Free one. Its service limits were far above the Always Free allowance (41 Arm OCPUs per availability domain), so a quota policy held the test compartment to the allowance. Treat every create as billable until a guard says otherwise.

1. **Budget alert.** In the console open Billing and Cost Management, then Budgets, and create a small monthly budget (for example 10 USD) on the tenancy. Add alert rules on actual spend at 1 percent and 50 percent and on forecast spend at 100 percent, sent to your email. The 1 percent rule tells you the first time anything bills. A budget only warns, it does not stop spending.
2. **Compartment quota policy.** Create a compartment for the VM (do not use the root compartment), then a quota policy in the root compartment that caps Arm compute there. The syntax is `set|zero|unset <family> quota <name> [to N] in compartment <name> [where request.ad = '<AD>']`, see Oracle's [quota policy syntax](https://docs.oracle.com/en-us/iaas/Content/Quotas/Concepts/quota_policy_syntax.htm). The quota names for the Arm shape are listed in Oracle's quota reference for compute, check them there. Cap Arm at 2 OCPUs and 12 GB of memory. A quota without an availability domain condition applies per availability domain, so use the condition to allow one domain and set the others to zero. Cap block storage at your boot volume plus a margin, well under the 200 GB total.

A quota is the hard guard. Service limits always win over quotas, and the most restrictive policy applies. Read the policy back in the console after you save it. Creating the quota policy and the budget in the console was done for the test run. The exact statement text is not reproduced here, so write it from Oracle's reference and check it by trying a create over the cap in the console (not yet verified as a documented step).

## What the free Arm shape gives you

Oracle's page says an Always Free tenancy gets the equivalent of 2 OCPUs and 12 GB of memory on the `VM.Standard.A1.Flex` shape, which you can split across instances. It also says boot volumes and block volumes share a 200 GB total, and that outbound data is 10 TB per month (all per the page linked above). Older guides quote 4 OCPUs and 24 GB. Do not assume that: the console shows what your tenancy actually has.

Check the region before you create. Your home region is fixed at signup, and Oracle's pages list regions where the Arm shape is not offered. Pick the availability domain with Arm capacity in your home region.

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
| `0.0.0.0/0` | UDP | 51820 | Mesh (WireGuard), only if you add more nodes |
| `0.0.0.0/0` | TCP | 22 | SSH (usually present by default) |

Port 8080 (the dashboard) is not in this table on purpose. The safe default is to leave it closed in the VCN and reach the dashboard through an SSH tunnel, see [First access to the dashboard](#first-access-to-the-dashboard). If you open TCP 8080, restrict the source CIDR to your own IP (`203.0.113.7/32` style), and remove the rule when you are done.

Add rules for any app port you publish directly, for example TCP 25565 for a Minecraft Java server, see [Game servers](/game-servers). Add `::/0` rules only if your VCN has IPv6.

## 3. Open the host firewall (layer two)

Opening the VCN is not enough. The images also carry a host firewall, and it is a separate switch. Which one depends on the image:

- Ubuntu images from Oracle commonly ship an iptables ruleset that allows SSH and rejects the rest at the end of the `INPUT` chain. `ufw` is usually not installed.
- Oracle Linux images use firewalld.

Look before you change anything:

```bash
sudo iptables -L INPUT -v -n --line-numbers
which ufw firewalld netfilter-persistent
```

On the stock Ubuntu image used in a test run (Ubuntu 24.04, `aarch64`, `iptables` v1.8.10 on the nf_tables backend, no `ufw` or `firewalld`), `INPUT` held these rules: 1 accept established, 2 accept ICMP, 3 accept loopback, 4 accept new TCP 22, 5 `REJECT --reject-with icmp-host-prohibited`. Yours may differ, so read your own listing first.

The installer warns when it finds an active firewalld or ufw. For `iptables`, insert allow rules ahead of the final reject. The position must sit before the `REJECT` line in your listing. On the image above, positions 5 and 6 worked:

```bash
sudo iptables -I INPUT 5 -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -p tcp --dport 443 -j ACCEPT
sudo iptables -I INPUT 7 -p udp --dport 443 -j ACCEPT
sudo iptables -I INPUT 8 -p udp --dport 51820 -j ACCEPT   # only for multi-node
sudo netfilter-persistent save
```

The UDP 51820 rule is the same insert pattern but was not run on the test VM (not yet verified). Skip it for a single node.

`netfilter-persistent save` writes `/etc/iptables/rules.v4`. On the test image the package was already installed, and the rules were still present after a reboot. Without the save, the rules are lost on reboot.

The installer's port 443 warning only mentions ufw and firewalld. On this image the commands above are the fix.

If you use firewalld (Oracle Linux), use `sudo firewall-cmd --permanent --add-port=80/tcp` (and 443) followed by `sudo firewall-cmd --reload`. That path was not part of the test run.

The go-live checklist in the dashboard can read the host firewall and open ports 80 and 443 for you when the control plane runs as root, and it shows Oracle Cloud steps for the VCN layer. It never edits existing rules. See [Go live](/go-live) and [Host firewall](/host-firewall).

### Which layer is blocking you

From outside the VM, the failure looks different per layer:

| What you see | Layer |
| --- | --- |
| Timeout | The VCN security list or network security group dropped the packet |
| Connection refused | The host answered. Either nothing listens on the port, or the host `REJECT` rule answered for it |

A host `REJECT` is not a silent drop, so it shows as refused, and some clients (macOS `nc` for one) report `host-prohibited` as refused too. Refused alone cannot tell "nothing is listening" from "host firewall reject". The counters can: run `sudo iptables -L INPUT -v -n --line-numbers` before and after a connection attempt. If the packet count on the `REJECT` rule grows, the host firewall is the blocker. Use `ss -tlnp` to check that something listens.

### Docker published ports and host services

On the test image, a container published with `docker run -p 25565:80 nginx:alpine` was reachable from outside with only the VCN rule open. There was no host `INPUT` rule for it and the `REJECT` counter did not move. Docker forwards published ports, so they skip `INPUT` on this image. Services that listen on the host itself (the dashboard on 8080, ingress on 80 and 443) do need an `INPUT` accept. This was tested for TCP only. UDP was not tested, because the test VCN had no UDP rule, so check UDP ports separately from outside, see [UDP published ports](/udp-ports).

## 4. Install Levelrail

SSH in and run the installer:

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

The installer installs Docker if it is missing. Measured once on a 1 OCPU / 6 GB Arm VM, 2026-11-01: the install took 67 seconds, the control plane used about 160 to 176 MB of resident memory, and used RAM on the machine was 549 MB at 3 minutes and 646 MB at about 17 minutes, idle, with Docker installed and no apps. This is one measurement, not a guarantee.

## First access to the dashboard

The installer prints `http://<public-ip>:8080`. With the VCN rules from step 2 that address times out, because TCP 8080 is not open. Port 80 does not help either: it answers with a 308 redirect to HTTPS, and HTTPS has no certificate until a certificate request succeeds. A browser or `openssl` fails at the TLS handshake in that state, there is no warning page to click through. The installer's note about a self-signed certificate warning on the `sslip.io` address did not match what the test run saw.

Use an SSH tunnel as the default:

```bash
ssh -L 8080:localhost:8080 ubuntu@<public-ip>
```

Leave it running and open `http://localhost:8080` in your browser, using the setup link from the installer summary (replace the host with `localhost`). Nothing extra is exposed to the internet.

The alternative is to open TCP 8080 in the VCN for a short time with the source set to your own IP, then delete the rule.

HTTPS needs a domain whose DNS points at the VM's public IP, with TCP 80 and 443 open in both layers, so the certificate request can succeed. Set the domain in the dashboard, see [Go live](/go-live) and [Domains and ingress](/domains-and-ingress). Until then, use the tunnel.

### HTTPS with an sslip.io host

With no domain, use the free `sslip.io` name for your IP. The IP `203.0.113.7` becomes `203-0-113-7.sslip.io`, which resolves to that IP with no DNS record. Open TCP 80 and 443 in both layers (steps 2 and 3), then set the dashboard's domain to that name as described in [Domains and ingress](/domains-and-ingress#zero-dns-setup-sslipio-hostnames-and-one-click-https). The certificate request needs port 80 reachable from the internet.

Issuing a certificate for an `sslip.io` name on this VM has not been run yet (not yet verified). Public certificate authorities rate limit shared domains such as `sslip.io`, so a request can fail for reasons outside your VM. A real domain avoids that.

### First login from the CLI on the server

`levelrail-cli readiness` and the other commands return `401 not signed in` until you log in, and the dashboard is the usual place to start. For a headless server, the password login from the CLI over plain HTTP needs new-device approval, even for the first admin. The hint it prints mentions `levelrail-cli auth approve`, which needs a token you do not have yet. What worked on the host:

```bash
levelrail-cli auth login --api-url http://localhost:8080 --username <name> --password <password>
# it prints a sign-in id and a line "Type this number"
sudo levelrail approve-signin <id> --match <number>
```

Run the approve command in a second SSH session while the login waits, with `<number>` taken from the "Type this number" line. After that, `levelrail-cli readiness` shows which ports answer from outside. If 80 or 443 show as blocked while the host firewall looks right, recheck the VCN rules from step 2. The single-use **Connect your CLI** code in the dashboard avoids this approval, see [Installing Levelrail](/installing).

Then follow [Installing Levelrail](/installing) and [Go live](/go-live).

## Arm and app images

An app runs on this VM only if its container image has a `linux/arm64` build. Many do, some do not. The template catalog does not guarantee every template works on Arm. Before you install a template, check the image's published platforms (for example with `docker buildx imagetools inspect <image:tag>`), and see [Template catalog](/template-catalog) and [Templates status](/templates-status), which lists templates known to fail on Arm. If an image has no arm64 build you will see a pull error or an `exec format error`.

A deploy of an image with no arm64 build now fails up front with a `PlatformMismatch` reason in the deploy trace, naming the image architecture and the node, unless the VM has qemu binfmt emulation registered for it (then the deploy goes ahead with a warning that it runs under emulation and may be slow). To allow x86 images, install emulation, for example `docker run --privileged --rm tonistiigi/binfmt --install amd64`. `APP_PLATFORM_MISMATCH` (`refuse-when-no-emulation`, `warn` or `refuse`) changes the policy; see [Environment variables](/environment-variables).

## Idle reclamation

Oracle's page says idle Always Free compute instances may be reclaimed. It defines idle as a 7-day period in which the 95th percentile CPU use is under 20 percent, network use is under 20 percent, and memory use is under 20 percent (the memory test applies to A1 shapes only). That is Oracle's rule as published at the link above. It can change, and the page does not describe in detail what reclaiming means for your instance.

What this means for you:

- A lightly used personal server can meet those conditions. Levelrail does not change that.
- Oracle's page says that after you upgrade to Pay As You Go it does not charge for Always Free resources, only for usage above the free limits. Whether to upgrade is your call. Read Oracle's terms and set a budget alert if you do. This page does not promise any particular outcome.
- The honest way to stay above the thresholds is to run real work on the instance. Check the instance's Monitoring graphs for CPU, network and memory over 7 days and compare them to the three limits. If your real use is lower, plan for the instance to be reclaimed, or upgrade on purpose.
- Levelrail does not include a keep-alive or load generator, and this guide does not suggest one.
- Keep backups off the VM, see [Backup targets and volume backups](/backups-and-storage), so a lost instance does not mean lost data.

## Clean up so nothing keeps billing

Delete in this order in the console, in the compartment you made for this. A stopped instance still bills for its boot volume.

1. Terminate the instance and tick the option to delete the boot volume.
2. Delete any block volumes, volume backups and reserved public IPs you created.
3. Delete the subnet, then the route table rules and internet gateway, then the VCN.
4. Delete the compartment once it is empty, then the quota policy and the budget if you no longer want them.
5. Next month, open Billing and Cost Management, then Cost Analysis, and confirm the spend is zero.

The commands for each delete were not run through to the end in the test (not yet verified). Do not delete the quota policy and budget before the other resources are gone.

## Related

- [Install on a cloud VM](/install-on-cloud)
- [Installing Levelrail](/installing)
- [Go live](/go-live)
- [Host firewall](/host-firewall)
- [Game servers](/game-servers)
