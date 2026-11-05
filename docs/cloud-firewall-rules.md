---
title: "Cloud firewall rules"
sidebarTitle: "Cloud firewall rules"
description: "Copy-paste cloud firewall rules for DigitalOcean, Hetzner, AWS, Google Cloud, Oracle Cloud and Azure: ports 80 and 443, and optionally 8080 from your own IP."
"og:url": "https://docs.levelrail.com/cloud-firewall-rules"
---

The installer opens only the host firewall (ufw) ports, and only when you pass `LEVELRAIL_CONFIGURE_UFW=1`. It never changes the cloud firewall, security group or network rules of your provider. You add those yourself, once per server.

The dashboard shows the commands for the host firewall backend it detects (ufw, firewalld or iptables). The doctor check and the host firewall card in Settings report what is blocked. Neither can see your provider's cloud firewall, see [Host firewall](/host-firewall) and [Doctor](/doctor).

## The rules

| Port | Protocol | Source | Needed for |
| --- | --- | --- | --- |
| 80 | TCP | Anywhere | Certificate issuance and the redirect to HTTPS |
| 443 | TCP | Anywhere | HTTPS for the dashboard and your apps |
| 443 | UDP | Anywhere | HTTP/3 (optional) |
| 8080 | TCP | Your IP only (optional) | Dashboard over plain HTTP until HTTPS is on |
| 22 | TCP | Your IP | SSH, usually open already |

Skip the 8080 rule if you use an SSH tunnel. See [Dashboard port 8080](/installing#dashboard-port-8080).

In every command below, replace `<your-ip>` with your public address, for example `203.0.113.7`. Replace names and IDs in angle brackets with your own.

## DigitalOcean

Console: **Networking, Firewalls, Create Firewall**. Add the inbound rules from the table and apply the firewall to the Droplet.

```bash
doctl compute firewall create \
  --name levelrail \
  --droplet-ids <droplet-id> \
  --inbound-rules "protocol:tcp,ports:22,address:<your-ip>/32 protocol:tcp,ports:80,address:0.0.0.0/0,address:::/0 protocol:tcp,ports:443,address:0.0.0.0/0,address:::/0 protocol:udp,ports:443,address:0.0.0.0/0,address:::/0 protocol:tcp,ports:8080,address:<your-ip>/32" \
  --outbound-rules "protocol:tcp,ports:all,address:0.0.0.0/0,address:::/0 protocol:udp,ports:all,address:0.0.0.0/0,address:::/0 protocol:icmp,address:0.0.0.0/0,address:::/0"
```

A Droplet firewall blocks anything it has no rule for, so the outbound rules matter: the server needs them to pull images and reach Let's Encrypt.

## Hetzner

Console: **Firewalls, Create Firewall**, add the inbound rules, then **Apply to** your server.

```bash
hcloud firewall create --name levelrail
hcloud firewall add-rule levelrail --direction in --protocol tcp --port 80 --source-ips 0.0.0.0/0 --source-ips ::/0
hcloud firewall add-rule levelrail --direction in --protocol tcp --port 443 --source-ips 0.0.0.0/0 --source-ips ::/0
hcloud firewall add-rule levelrail --direction in --protocol udp --port 443 --source-ips 0.0.0.0/0 --source-ips ::/0
hcloud firewall add-rule levelrail --direction in --protocol tcp --port 8080 --source-ips <your-ip>/32
hcloud firewall add-rule levelrail --direction in --protocol tcp --port 22 --source-ips <your-ip>/32
hcloud firewall apply-to-resource levelrail --type server --server <server-name>
```

## AWS

Console: **EC2, Security Groups**, pick the group attached to the instance, **Edit inbound rules**.

```bash
aws ec2 authorize-security-group-ingress --group-id <sg-id> --protocol tcp --port 80 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-id <sg-id> --protocol tcp --port 443 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-id <sg-id> --protocol udp --port 443 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-id <sg-id> --protocol tcp --port 8080 --cidr <your-ip>/32
```

Add `--region <region>` if it is not your default. The commands add IPv4 rules only.

## Google Cloud

Console: **VPC network, Firewall, Create firewall rule**. Direction ingress, action allow, and a target tag you also put on the instance.

```bash
gcloud compute firewall-rules create levelrail-web \
  --network=default \
  --direction=INGRESS \
  --allow=tcp:80,tcp:443,udp:443 \
  --source-ranges=0.0.0.0/0 \
  --target-tags=levelrail-control-plane

gcloud compute firewall-rules create levelrail-dashboard \
  --network=default \
  --direction=INGRESS \
  --allow=tcp:8080 \
  --source-ranges=<your-ip>/32 \
  --target-tags=levelrail-control-plane
```

The instance needs the network tag `levelrail-control-plane`. The `--network` value is a placeholder if your project uses a custom VPC.

## Oracle Cloud

Console: **Networking, Virtual cloud networks**, open your VCN, then the subnet's **Security List** (or the network security group on the instance), **Add Ingress Rules**. Source CIDR `0.0.0.0/0`, IP protocol TCP, destination port 80, then again for 443 and for UDP 443. For 8080 use `<your-ip>/32`.

Oracle images also carry a host firewall, which is a second switch. The full steps are in [Install on Oracle Cloud](/install-on-oracle-cloud). For the `oci` CLI, check the provider docs for the flags: updating a security list replaces its whole rule set, so the console is safer for a first setup.

## Azure

Console: **Virtual machines**, your VM, **Networking, Network settings, Create port rule, Inbound port rule**. This edits the network security group (NSG) of the VM.

```bash
az network nsg rule create \
  --resource-group <resource-group> \
  --nsg-name <nsg-name> \
  --name allow-levelrail-web \
  --priority 100 \
  --direction Inbound \
  --access Allow \
  --protocol Tcp \
  --source-address-prefixes '*' \
  --destination-port-ranges 80 443

az network nsg rule create \
  --resource-group <resource-group> \
  --nsg-name <nsg-name> \
  --name allow-levelrail-http3 \
  --priority 110 \
  --direction Inbound \
  --access Allow \
  --protocol Udp \
  --source-address-prefixes '*' \
  --destination-port-ranges 443

az network nsg rule create \
  --resource-group <resource-group> \
  --nsg-name <nsg-name> \
  --name allow-levelrail-dashboard \
  --priority 120 \
  --direction Inbound \
  --access Allow \
  --protocol Tcp \
  --source-address-prefixes <your-ip>/32 \
  --destination-port-ranges 8080
```

Priorities must be unique in the NSG. Change them if a rule already uses 100, 110 or 120.

## Check that it worked

From another machine, not the server:

```bash
curl -I http://<server-ip>
```

Any HTTP response means port 80 is open. A timeout means a firewall is still blocking it. The CLI flags above were not run against real accounts for this page, so check the provider docs if a command is rejected.

## Related

- [Install on a cloud VM](/install-on-cloud)
- [Install on Oracle Cloud](/install-on-oracle-cloud)
- [Host firewall](/host-firewall)
- [Zero to HTTPS](/zero-to-https)
- [Go live checklist](/go-live)
