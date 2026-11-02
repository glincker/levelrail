---
title: "Get a certificate behind CGNAT with DNS-01 and dynamic DNS"
icon: "/assets/icons/globe.svg"
sidebarTitle: "DNS-01 behind CGNAT"
description: Get trusted certificates for a home server behind carrier-grade NAT, where the certificate authority cannot reach the server on port 80, using DNS-01 and dynamic DNS.
"og:url": "https://docs.levelrail.com/tutorials/dns-01-behind-cgnat"
---

Many home and mobile connections sit behind carrier-grade NAT (CGNAT). The provider shares one public IPv4 address between many customers, and you cannot forward a port from the internet to your server. The certificate authority then cannot reach port 80 to check HTTP-01, so a normal certificate request fails.

DNS-01 avoids that check. The certificate authority asks for a TXT record in your DNS zone instead, and Levelrail writes that record through your DNS provider's API. Dynamic DNS keeps the name pointed at the current address. Read the limit at the end before you rely on this for public traffic.

## What works and what does not

| Need | Works behind CGNAT | Why |
| --- | --- | --- |
| A trusted certificate for a hostname | Yes, with DNS-01 | The check uses DNS, not an inbound connection. |
| The HTTP-01 check | No | The authority needs an inbound connection on port 80. |
| Browsers reaching the site from the internet | Not by DNS alone | The name points at the shared address, which accepts no inbound connection to your server. |

So DNS-01 and dynamic DNS give you a valid certificate and a name that tracks your address. They do not make the server reachable from outside. For that, see [Public access](#public-access-after-the-certificate) below.

## Before you start

- A DNS zone you control, with a provider Levelrail supports for DNS-01: Cloudflare, Route53 or Google Cloud DNS.
- An API token for that provider with permission to edit DNS records in the zone.
- A name for the server in that zone, for example `home.example.com`.

## Step 1: Connect the DNS provider

Cloudflare is the simplest to start with:

```bash
levelrail-cli domains cloudflare-dns set --cf-api-token <token>
levelrail-cli domains cloudflare-dns get
```

The token needs `Zone:DNS:Edit` and `Zone:Zone:Read` on the zone you use. The token is stored encrypted and is not returned by the API.

## Step 2: Turn on DNS-01 for every host

Single hosts use HTTP-01 by default, which fails behind CGNAT. Turn on DNS-01 for all hosts:

```bash
levelrail-cli settings ingress dns01 enable
levelrail-cli settings ingress dns01 show
```

DNS-01 for single hosts is not yet verified against a real certificate authority. Check the result on your own zone before you depend on it (pending run).

## Step 3: Keep the name on your current address

Dynamic DNS checks the server's public address on a timer and updates the record when it changes. The example uses DuckDNS:

```bash
levelrail-cli settings ddns set --provider duckdns --domains home --target -
levelrail-cli settings ddns enable
levelrail-cli settings ddns status
```

The DDNS updater is unit tested but has not yet been run against a live DuckDNS account, so check the status line after the first update (pending run).

Behind CGNAT, the address the updater publishes is the shared one. Dynamic DNS keeps the name correct for your provider's view of your address, but it does not make a port reachable.

## Step 4: Check the certificate

Open the Domains page and check the host's certificate status. It should show as issued once the TXT record check passes. If it shows a DNS error, check that the zone is the one the token can edit, and that the record name is not blocked by a CNAME at that name.

## Public access after the certificate

A trusted certificate is only half of public access. Pick one way in:

- A public IPv6 address, if your provider gives you one and your firewall allows the ports. Then the name can point at the IPv6 address.
- A tunnel from a provider that does not need an inbound port. Levelrail's own tunnel setting is experimental and covers the control plane, not app traffic (see the [CLI reference](/cli-reference)). For a proxy in front of your apps, see [Run behind Cloudflare](/behind-cloudflare).
- A small public server or VPS in front of the home server, which forwards to it over a private link. See [Run behind an existing proxy](/behind-an-existing-proxy).

Levelrail does not open an inbound route for you through CGNAT. Confirm any route from outside your own network before you point users at it (pending run).

## Where to go next

- [Dynamic DNS](/dynamic-dns): every setting and the status fields.
- [Domains and ingress](/domains-and-ingress): the certificate options and what each check means.
