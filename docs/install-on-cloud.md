---
title: "Install on a cloud VM"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Install on a cloud VM"
description: Copy-paste user data and startup scripts that install the Levelrail control plane on a fresh AWS, Google Cloud, Azure, DigitalOcean or Hetzner VM, with the firewall ports to open.
"og:url": "https://docs.levelrail.com/install-on-cloud"
---

This page is for a fresh VM where you want the control plane to install itself on first boot. The install itself is the same `install.sh` described in [Installing Levelrail](/installing). The snippets below only pass that command to each cloud's boot hook.

Read [Installing Levelrail](/installing) first for the requirements. Use an Ubuntu 24.04 or Debian 12 image, `amd64` or `arm64`, with at least 1 vCPU, 1 GB RAM and 10 GB disk. 2 vCPU, 2 GB RAM and 20 GB disk is more comfortable.

## The install script

Every snippet below runs the same script. It skips the install when the `levelrail.service` unit already exists, so a reboot or a second boot does not run the installer again.

```bash
#!/bin/bash
# Install the Levelrail control plane once. Safe to run on every boot.
systemctl cat levelrail.service >/dev/null 2>&1 && exit 0
curl -fsSL https://levelrail.com/install.sh | sh
```

Cloud boot hooks run as root, so the script does not need `sudo`. Pass `--domain` or `--profile` to the same `sh` call when you need them, for example `curl -fsSL https://levelrail.com/install.sh | sh -s -- --profile standard`. The flags are listed in `install.sh --help` and in [Installing Levelrail](/installing).

## Firewall ports

Open these inbound ports in the cloud firewall, security group or network rules before you test the dashboard.

| Port | Protocol | Why |
| --- | --- | --- |
| 80 | TCP, from anywhere | Let's Encrypt HTTP-01 and the HTTP ingress |
| 443 | TCP, from anywhere | HTTPS ingress for your apps and the dashboard |
| 8080 | TCP, from your admin IP only | Dashboard and API until HTTPS is set up. The installer picks the next free port if 8080 is taken and prints the real one |

Keep SSH (22) open to your own address so you can read the setup token. Optionally open `443/udp` for HTTP/3. Agent-only nodes need no inbound ports at all, see [Multi-node](/multi-node).

<Warning>
The first admin setup token is printed at the end of the install. It is never written to the service log; print it again with `sudo levelrail setup-token`. Treat it as a secret. Read it over SSH, use it once, and do not paste it into tickets or chat. Back up `<data dir>/master.key` as soon as the install finishes, as the installer reminds you. Without that key, encrypted secrets cannot be read after a restore.
</Warning>

## AWS EC2

Use the script as EC2 user data. It runs once, as root, on first boot. Create the security group with ports 80 and 443 open to the internet and 8080 open to your admin address only.

```bash
aws ec2 run-instances \
  --image-id <ubuntu-24.04-ami-id> \
  --instance-type t3.small \
  --key-name <your-key-pair> \
  --security-group-ids <sg-id> \
  --user-data file://install-levelrail.sh
```

The image id and key pair are placeholders. Find the Ubuntu 24.04 AMI for your region in the EC2 console or from Canonical's published images.

The Terraform example below shows the same thing as code. It is an example only and has not been run against a real AWS account.

```hcl
# Example only. Not run against a real AWS account.
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "region" {
  type    = string
  default = "eu-west-1"
}

variable "admin_cidr" {
  description = "Your public address in CIDR form, for the dashboard port 8080"
  type        = string
}

provider "aws" {
  region = var.region
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
}

resource "aws_security_group" "levelrail" {
  name        = "levelrail-control-plane"
  description = "Levelrail control plane ingress"

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = [var.admin_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "control_plane" {
  ami                         = data.aws_ami.ubuntu.id
  instance_type               = "t3.small"
  vpc_security_group_ids      = [aws_security_group.levelrail.id]
  user_data                   = file("${path.module}/install-levelrail.sh")
  user_data_replace_on_change = true

  root_block_device {
    volume_size = 20
  }

  tags = {
    Name = "levelrail-control-plane"
  }
}
```

The example uses the default VPC and has no SSH rule or key pair, so add those before you rely on it. The `user_data_replace_on_change` flag replaces the instance when the script changes, which destroys its data. Remove it if you want to keep an existing instance.

## Google Cloud Compute Engine

Pass the script as a startup script. Google Cloud runs startup scripts on every boot, which is why the script above checks for the existing unit first.

```bash
gcloud compute firewall-rules create levelrail-web \
  --network=default \
  --allow=tcp:80,tcp:443 \
  --target-tags=levelrail-control-plane

gcloud compute instances create levelrail-control-plane \
  --zone=europe-west1-b \
  --machine-type=e2-small \
  --image-family=ubuntu-2404-lts-amd64 \
  --image-project=ubuntu-os-cloud \
  --boot-disk-size=20GB \
  --tags=levelrail-control-plane \
  --metadata-from-file=startup-script=install-levelrail.sh
```

For the dashboard port, add a second rule that allows `tcp:8080` from your own address with `--source-ranges=<your-ip>/32`. The `--network` value is a placeholder if your project uses a custom VPC. Startup script logs are in `journalctl -u google-startup-scripts` on the instance.

## Azure VM

Pass the script as custom data. Azure encodes it for you when you give the file to the CLI. Use a `cloud-config` file so the same snippet also works in other tools.

```yaml
#cloud-config
write_files:
  - path: /usr/local/sbin/install-levelrail.sh
    permissions: "0755"
    content: |
      #!/bin/bash
      systemctl cat levelrail.service >/dev/null 2>&1 && exit 0
      curl -fsSL https://levelrail.com/install.sh | sh
runcmd:
  - [ /usr/local/sbin/install-levelrail.sh ]
```

```bash
az vm create \
  --resource-group <your-resource-group> \
  --name levelrail-control-plane \
  --image Canonical:ubuntu-24_04-lts:server:latest \
  --size Standard_B2s \
  --admin-username azureuser \
  --generate-ssh-keys \
  --custom-data cloud-init.yaml

az vm open-port \
  --resource-group <your-resource-group> \
  --name levelrail-control-plane \
  --port 80,443 \
  --priority 1010
```

Open port 8080 in the network security group to your own address only, with a rule that has a higher priority than the open rules. Custom data is in `/var/lib/cloud/instance/user-data.txt` and the install log is in `/var/log/cloud-init-output.log`.

## DigitalOcean Droplet

Paste the script into the Droplet's **User data** field in the control panel, or pass it with `doctl`.

```bash
doctl compute droplet create levelrail-control-plane \
  --region fra1 \
  --size s-1vcpu-2gb \
  --image ubuntu-24-04-x64 \
  --ssh-keys <your-key-fingerprint> \
  --user-data-file install-levelrail.sh
```

Create a Cloud Firewall with inbound TCP 80 and 443 from all addresses and TCP 8080 from your own address. Attach it to the Droplet. The install log is in `/var/log/cloud-init-output.log`.

## Hetzner Cloud server

Paste the script into the server's **Cloud config** field in the Hetzner console, or pass it with `hcloud`.

```bash
hcloud server create \
  --name levelrail-control-plane \
  --type cx22 \
  --image ubuntu-24.04 \
  --location fsn1 \
  --ssh-key <your-key-name> \
  --user-data-from-file install-levelrail.sh
```

Create a Hetzner Cloud Firewall with inbound TCP 80 and 443 from all addresses and TCP 8080 from your own address, then apply it to the server. The install log is in `/var/log/cloud-init-output.log`.

## After the install

1. Read the dashboard URL and the setup token from the install output over SSH, or run `sudo levelrail setup-token`.
2. Open `http://<public-ip>:8080/login#setup=<token>` once, create the admin, then set a domain with TLS on the dashboard as described in [Domains and ingress](/domains-and-ingress#the-dashboards-own-domain).
3. Back up `master.key` from the data directory, then close port 8080 to the public.
4. Run `levelrail-cli doctor` on the server to check Docker, disk and the ports.

The cloud CLI and Terraform snippets on this page have not been run end to end against real accounts. See [Cloud integrations](/cloud-integrations) for what is built and what is planned.
