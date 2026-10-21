package hostprovider

// PortRule is one inbound rule to create.
type PortRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}

// Guidance is the exact manual procedure for one provider's firewall.
type Guidance struct {
	ID      ID         `json:"id"`
	Name    string     `json:"name"`
	Source  string     `json:"source"`
	DocsURL string     `json:"docs_url"`
	Ports   []PortRule `json:"ports"`
	Steps   []string   `json:"steps"`
	Note    string     `json:"note,omitempty"`
}

func webRules() []PortRule {
	return []PortRule{{80, "tcp", "0.0.0.0/0 and ::/0"}, {443, "tcp", "0.0.0.0/0 and ::/0"}}
}

// For returns the guidance for id, tagged with how it was detected.
func For(id ID, source string) Guidance {
	g := Guidance{ID: id, Source: source, Ports: webRules()}
	switch id {
	case DigitalOcean:
		g.Name = "DigitalOcean"
		g.DocsURL = "https://docs.digitalocean.com/products/networking/firewalls/how-to/configure-rules/"
		g.Steps = []string{
			"Open Networking, then Firewalls in the DigitalOcean control panel.",
			"If a Cloud Firewall is attached to this Droplet, open it and choose Inbound Rules, Edit. If there is none, there is nothing to open.",
			"Add the HTTP rule (TCP 80) and the HTTPS rule (TCP 443) with sources All IPv4 and All IPv6, then save.",
		}
	case AWS:
		g.Name = "Amazon EC2"
		g.DocsURL = "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/changing-security-group.html"
		g.Steps = []string{
			"Open EC2, then Instances, select this instance and open the Security tab.",
			"Open its security group, then Inbound rules, Edit inbound rules.",
			"Add HTTP (TCP 80) and HTTPS (TCP 443) with source 0.0.0.0/0, and ::/0 if the domain has an AAAA record, then save.",
		}
		g.Note = "A custom network ACL on the subnet must also allow these ports in both directions."
	case GCP:
		g.Name = "Google Compute Engine"
		g.DocsURL = "https://cloud.google.com/firewall/docs/using-firewalls"
		g.Steps = []string{
			"Open VPC network, then Firewall in the Google Cloud console and choose Create firewall rule.",
			"Set Direction to Ingress, Action to Allow, Targets to the network tag of this VM (or All instances), Source IPv4 ranges to 0.0.0.0/0.",
			"Under Protocols and ports choose TCP with 80,443, then create the rule. Ticking Allow HTTP and Allow HTTPS traffic on the VM also works.",
		}
	case Hetzner:
		g.Name = "Hetzner Cloud"
		g.DocsURL = "https://docs.hetzner.com/cloud/firewalls/getting-started/creating-a-firewall/"
		g.Steps = []string{
			"Open the project in the Hetzner Cloud Console, then Firewalls.",
			"If a firewall is applied to this server, open it. If none is applied, nothing is blocking at the provider.",
			"Add inbound rules for TCP 80 and TCP 443 with sources Any IPv4 and Any IPv6, then save.",
		}
	case Azure:
		g.Name = "Microsoft Azure"
		g.DocsURL = "https://learn.microsoft.com/azure/virtual-network/manage-network-security-group"
		g.Steps = []string{
			"Open the virtual machine in the Azure portal, then Networking, Network settings.",
			"Choose Create port rule, Inbound port rule.",
			"Allow destination ports 80 and 443 with protocol TCP and source Any (or add one rule per port), then add the rule.",
		}
	case Oracle:
		g.Name = "Oracle Cloud"
		g.DocsURL = "https://docs.oracle.com/en-us/iaas/Content/Network/Concepts/securityrules.htm"
		g.Steps = []string{
			"Open the instance, then its subnet and the subnet's Security List (or the Network Security Group on the VNIC).",
			"Add ingress rules for TCP 80 and TCP 443 with source CIDR 0.0.0.0/0.",
		}
		g.Note = "Oracle images also ship an iptables rule set that rejects other ports, so the host firewall step above matters as much as this one."
	case Vultr:
		g.Name = "Vultr"
		g.DocsURL = "https://docs.vultr.com/vultr-firewall"
		g.Steps = []string{
			"Open Products, then Firewall in the Vultr dashboard.",
			"If a firewall group is linked to this server, open it. If none is linked, nothing is blocking at the provider.",
			"Add rules for HTTP (TCP 80) and HTTPS (TCP 443) with source 0.0.0.0/0, and ::/0 for IPv6, then save.",
		}
	case Linode:
		g.Name = "Linode (Akamai)"
		g.DocsURL = "https://www.linode.com/docs/products/networking/cloud-firewall/guides/manage-firewall-rules/"
		g.Steps = []string{
			"Open Firewalls in Akamai Cloud Manager.",
			"If a Cloud Firewall is assigned to this Linode, open it and choose Add an Inbound Rule.",
			"Add HTTP (TCP 80) and HTTPS (TCP 443) with sources All IPv4 and All IPv6, then save.",
		}
	}
	return g
}
