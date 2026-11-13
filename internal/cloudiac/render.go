package cloudiac

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
)

// MainFile is the one configuration file a rendered module carries.
const MainFile = "main.tf.json"

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// NetworkSpec is a VCN with one public subnet.
type NetworkSpec struct {
	Name         string
	CIDR         string
	IngressPorts []int
}

// BucketSpec is an object storage bucket. Buckets are protected from
// destroy unless Unprotected is set.
type BucketSpec struct {
	Name        string
	Unprotected bool
}

// InstanceSpec is a compute instance placed in a rendered network.
type InstanceSpec struct {
	Name               string
	Network            string
	Shape              string
	OCPUs              int
	MemoryGB           int
	AvailabilityDomain string
	ImageOCID          string
}

// OCISpec is the typed input for one OCI module. Provider credentials are
// not part of it; the engine supplies them per step.
type OCISpec struct {
	Name            string
	Region          string
	CompartmentOCID string
	Namespace       string
	Networks        []NetworkSpec
	Buckets         []BucketSpec
	Instances       []InstanceSpec
}

type obj = map[string]any

// RenderOCI renders the spec to a ModuleSpec holding Terraform JSON
// configuration. Output is deterministic and never carries credentials.
func RenderOCI(s OCISpec) (ModuleSpec, error) {
	if err := validateOCI(s); err != nil {
		return ModuleSpec{}, &Error{Kind: ErrInvalidSpec, Step: StepPlan, Message: err.Error(), Err: err}
	}
	res := map[string]obj{}
	add := func(typ, name string, body obj) {
		if res[typ] == nil {
			res[typ] = obj{}
		}
		res[typ][name] = body
	}
	for _, n := range s.Networks {
		renderNetwork(n, add)
	}
	for _, b := range s.Buckets {
		body := obj{
			"compartment_id": "${var.compartment_ocid}",
			"namespace":      "${var.object_namespace}",
			"name":           b.Name,
			"access_type":    "NoPublicAccess",
		}
		if !b.Unprotected {
			body["lifecycle"] = obj{"prevent_destroy": true}
		}
		add("oci_objectstorage_bucket", tfName(b.Name), body)
	}
	for _, i := range s.Instances {
		add("oci_core_instance", tfName(i.Name), obj{
			"compartment_id":      "${var.compartment_ocid}",
			"availability_domain": i.AvailabilityDomain,
			"display_name":        i.Name,
			"shape":               i.Shape,
			"shape_config":        obj{"ocpus": i.OCPUs, "memory_in_gbs": i.MemoryGB},
			"source_details":      obj{"source_type": "image", "source_id": i.ImageOCID},
			"create_vnic_details": obj{"subnet_id": "${oci_core_subnet." + tfName(i.Network) + ".id}", "assign_public_ip": true},
		})
	}
	vars := obj{
		"region":           obj{"type": "string"},
		"compartment_ocid": obj{"type": "string"},
	}
	values := map[string]string{"region": s.Region, "compartment_ocid": s.CompartmentOCID}
	if len(s.Buckets) > 0 {
		vars["object_namespace"] = obj{"type": "string"}
		values["object_namespace"] = s.Namespace
	}
	doc := obj{
		"terraform": obj{"required_providers": obj{"oci": obj{"source": "oracle/oci"}}},
		"variable":  vars,
		"provider":  obj{"oci": obj{"region": "${var.region}"}},
		"resource":  res,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ModuleSpec{}, &Error{Kind: ErrInternal, Step: StepPlan, Message: "render failed", Err: err}
	}
	return ModuleSpec{
		Name:      s.Name,
		Provider:  "oracle",
		Files:     map[string][]byte{MainFile: append(data, '\n')},
		Variables: values,
	}, nil
}

func renderNetwork(n NetworkSpec, add func(string, string, obj)) {
	k := tfName(n.Name)
	add("oci_core_vcn", k, obj{"compartment_id": "${var.compartment_ocid}", "cidr_blocks": []string{n.CIDR}, "display_name": n.Name})
	add("oci_core_internet_gateway", k, obj{"compartment_id": "${var.compartment_ocid}", "vcn_id": "${oci_core_vcn." + k + ".id}", "enabled": true})
	add("oci_core_route_table", k, obj{
		"compartment_id": "${var.compartment_ocid}",
		"vcn_id":         "${oci_core_vcn." + k + ".id}",
		"route_rules": []obj{{
			"destination":       "0.0.0.0/0",
			"destination_type":  "CIDR_BLOCK",
			"network_entity_id": "${oci_core_internet_gateway." + k + ".id}",
		}},
	})
	ports := append([]int(nil), n.IngressPorts...)
	sort.Ints(ports)
	rules := []obj{}
	for _, p := range ports {
		rules = append(rules, obj{"protocol": "6", "source": "0.0.0.0/0", "tcp_options": obj{"min": p, "max": p}})
	}
	add("oci_core_security_list", k, obj{
		"compartment_id":         "${var.compartment_ocid}",
		"vcn_id":                 "${oci_core_vcn." + k + ".id}",
		"ingress_security_rules": rules,
		"egress_security_rules":  []obj{{"protocol": "all", "destination": "0.0.0.0/0"}},
	})
	add("oci_core_subnet", k, obj{
		"compartment_id":    "${var.compartment_ocid}",
		"vcn_id":            "${oci_core_vcn." + k + ".id}",
		"cidr_block":        n.CIDR,
		"route_table_id":    "${oci_core_route_table." + k + ".id}",
		"security_list_ids": []string{"${oci_core_security_list." + k + ".id}"},
	})
}

// tfName maps a validated name to a Terraform identifier.
func tfName(n string) string {
	b := []byte(n)
	for i, c := range b {
		if c == '-' {
			b[i] = '_'
		}
	}
	return string(b)
}

func validateOCI(s OCISpec) error {
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("module name %q is invalid", s.Name)
	}
	if s.Region == "" || s.CompartmentOCID == "" {
		return fmt.Errorf("region and compartment are required")
	}
	if len(s.Buckets) > 0 && s.Namespace == "" {
		return fmt.Errorf("object storage namespace is required for buckets")
	}
	seen := map[string]bool{}
	claim := func(kind, name string) error {
		if !nameRe.MatchString(name) {
			return fmt.Errorf("%s name %q is invalid", kind, name)
		}
		key := kind + "/" + name
		if seen[key] {
			return fmt.Errorf("duplicate %s %q", kind, name)
		}
		seen[key] = true
		return nil
	}
	nets := map[string]bool{}
	for _, n := range s.Networks {
		if err := claim("network", n.Name); err != nil {
			return err
		}
		if _, err := netip.ParsePrefix(n.CIDR); err != nil {
			return fmt.Errorf("network %q has an invalid cidr", n.Name)
		}
		for _, p := range n.IngressPorts {
			if p < 1 || p > 65535 {
				return fmt.Errorf("network %q has an invalid port %d", n.Name, p)
			}
		}
		nets[n.Name] = true
	}
	for _, b := range s.Buckets {
		if err := claim("bucket", b.Name); err != nil {
			return err
		}
	}
	for _, i := range s.Instances {
		if err := claim("instance", i.Name); err != nil {
			return err
		}
		if !nets[i.Network] {
			return fmt.Errorf("instance %q references unknown network %q", i.Name, i.Network)
		}
		if i.Shape == "" || i.OCPUs < 1 || i.MemoryGB < 1 || i.AvailabilityDomain == "" || i.ImageOCID == "" {
			return fmt.Errorf("instance %q is missing shape, size, domain or image", i.Name)
		}
	}
	return nil
}
