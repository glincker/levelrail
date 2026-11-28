package provision

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/glincker/levelrail/internal/ocisign"
)

// maxOracleMetadataBytes is OCI's cap on the metadata map; checked before launch
// so an oversized bootstrap script fails with a clear message, not an opaque 400.
const maxOracleMetadataBytes = 32000

var (
	oracleOCIDRe   = regexp.MustCompile(`^ocid1\.[a-z0-9]+\.[a-z0-9]+\.[a-z0-9._-]*\.[a-z0-9]+$`)
	oracleRegionRe = regexp.MustCompile(`^[a-z]{2}-[a-z]+-[0-9]+$`)

	errOracleNoCapacity = errors.New("provision: oracle has no capacity in any allowed availability domain, retry later")
)

// OracleCredential is the API key material plus scope the driver needs.
type OracleCredential struct {
	TenancyOCID     string
	UserOCID        string
	Fingerprint     string
	PrivateKeyPEM   string
	CompartmentOCID string
	// SubnetOCID is required until the network slice can create one.
	SubnetOCID string
	Region     string
	// SessionToken switches signing to the browser session scheme; UserOCID and Fingerprint are then unused.
	SessionToken string
}

// OracleSettings are the tunables the later settings slice will feed live.
// Zero values fall back to the documented defaults.
type OracleSettings struct {
	FlexPresets         string // ocpus:memoryGB pairs, comma separated
	BootVolumeGB        int
	AvailabilityDomains []string // AD name suffixes, empty means all
	ImageOS             string
	ImageOSVersion      string
	// LabelKey prefixes the freeform tag that marks instances we launched.
	LabelKey string
	// AllowCompartmentScope lets the usage count fall back to the configured
	// compartment when the key cannot read the whole tenancy tree.
	AllowCompartmentScope bool
	// MaxBootVolumeGB refuses a create whose boot volume is larger; 0 is no cap.
	MaxBootVolumeGB int
}

func (s OracleSettings) withDefaults() OracleSettings {
	if s.FlexPresets == "" {
		s.FlexPresets = "1:6,2:12"
	}
	if s.BootVolumeGB <= 0 {
		s.BootVolumeGB = 50
	}
	if s.ImageOS == "" {
		s.ImageOS = "Canonical Ubuntu"
	}
	if s.ImageOSVersion == "" {
		s.ImageOSVersion = "24.04"
	}
	if s.LabelKey == "" {
		s.LabelKey = "levelrail-provision-id"
	}
	return s
}

// Oracle provisions OCI Core instances over signed REST calls, no SDK.
type Oracle struct {
	cred     OracleCredential
	settings OracleSettings
	signer   ocisign.Signer
	// iaasBase and identityBase are test seams; empty means derive from region.
	iaasBase     string
	identityBase string
	// onRegion is a test seam that sees every region a request is built for.
	onRegion func(string)
}

// NewOracle validates the credential and builds a driver for it.
func NewOracle(cred OracleCredential, settings OracleSettings) (*Oracle, error) {
	return newOracle(cred, settings, "", "", false)
}

// NewOracleWithEndpoints points the driver at explicit plain HTTP base URLs.
// Only Go callers can reach it, so tests can use a local fake without a
// config switch that could redirect signed requests.
func NewOracleWithEndpoints(cred OracleCredential, settings OracleSettings, iaasBase, identityBase string) (*Oracle, error) {
	return newOracle(cred, settings, iaasBase, identityBase, true)
}

func newOracle(cred OracleCredential, settings OracleSettings, iaasBase, identityBase string, insecure bool) (*Oracle, error) {
	session := cred.SessionToken != ""
	for field, v := range map[string]string{
		"tenancy": cred.TenancyOCID, "user": cred.UserOCID, "compartment": cred.CompartmentOCID,
	} {
		if session && field == "user" {
			continue
		}
		if !oracleOCIDRe.MatchString(v) {
			return nil, fmt.Errorf("provision: oracle %s OCID is not valid", field)
		}
	}
	if cred.SubnetOCID != "" && !oracleOCIDRe.MatchString(cred.SubnetOCID) {
		return nil, errors.New("provision: oracle subnet OCID is not valid")
	}
	if !oracleRegionRe.MatchString(cred.Region) {
		return nil, errors.New("provision: oracle region is not valid")
	}
	if cred.Fingerprint == "" && !session {
		return nil, errors.New("provision: oracle fingerprint is required")
	}
	key, err := parseOraclePrivateKey(cred.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	keyID := ocisign.KeyID(cred.TenancyOCID, cred.UserOCID, cred.Fingerprint)
	if session {
		keyID = ocisign.SessionKeyID(cred.SessionToken)
	}
	return &Oracle{
		cred:     cred,
		settings: settings.withDefaults(),
		signer: ocisign.Signer{
			KeyID:         keyID,
			Key:           key,
			AllowInsecure: insecure,
		},
		iaasBase:     iaasBase,
		identityBase: identityBase,
	}, nil
}

// SessionAuth reports a browser session login, which cannot create nodes.
func (o *Oracle) SessionAuth() bool { return o.cred.SessionToken != "" }

func parseOraclePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("provision: oracle private key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("provision: oracle private key must be RSA")
		}
		key = rk
	} else {
		return nil, errors.New("provision: oracle private key cannot be parsed")
	}
	minBits := 2048
	if n, err := strconv.Atoi(os.Getenv("APP_CLOUD_OCI_MIN_RSA_BITS")); err == nil && n > 0 {
		minBits = n
	}
	if key.N.BitLen() < minBits {
		return nil, errors.New("provision: oracle private key is too short")
	}
	return key, nil
}

func (o *Oracle) iaas(region string) *httpClient {
	if o.onRegion != nil {
		o.onRegion(region)
	}
	if o.iaasBase != "" {
		return newHTTPClientWithSigner(o.iaasBase, o.signer)
	}
	return newHTTPClientWithSigner("https://iaas."+region+".oraclecloud.com/20160918", o.signer)
}

func (o *Oracle) identity(region string) *httpClient {
	if o.identityBase != "" {
		return newHTTPClientWithSigner(o.identityBase, o.signer)
	}
	return newHTTPClientWithSigner("https://identity."+region+".oraclecloud.com/20160918", o.signer)
}

func (o *Oracle) regionOr(region string) string {
	if oracleRegionRe.MatchString(region) {
		return region
	}
	return o.cred.Region
}

// EffectiveRegion is the region a request with this region value acts in.
func (o *Oracle) EffectiveRegion(region string) string { return o.regionOr(region) }

// oracleRegionKeys maps the short region key found in instance OCIDs to the
// region name; an unlisted key makes callers search the subscribed regions.
var oracleRegionKeys = map[string]string{
	"iad": "us-ashburn-1", "phx": "us-phoenix-1", "ord": "us-chicago-1", "sjc": "us-sanjose-1",
	"fra": "eu-frankfurt-1", "lhr": "uk-london-1", "cwl": "uk-cardiff-1", "ams": "eu-amsterdam-1",
	"zrh": "eu-zurich-1", "arn": "eu-stockholm-1", "mrs": "eu-marseille-1", "cdg": "eu-paris-1",
	"mad": "eu-madrid-1", "mil": "eu-milan-1", "yyz": "ca-toronto-1", "yul": "ca-montreal-1",
	"nrt": "ap-tokyo-1", "kix": "ap-osaka-1", "icn": "ap-seoul-1", "yny": "ap-chuncheon-1",
	"bom": "ap-mumbai-1", "hyd": "ap-hyderabad-1", "syd": "ap-sydney-1", "mel": "ap-melbourne-1",
	"sin": "ap-singapore-1", "gru": "sa-saopaulo-1", "vcp": "sa-vinhedo-1", "scl": "sa-santiago-1",
	"bog": "sa-bogota-1", "jed": "me-jeddah-1", "dxb": "me-dubai-1", "auh": "me-abudhabi-1",
	"jnb": "af-johannesburg-1", "qro": "mx-queretaro-1", "mty": "mx-monterrey-1",
}

// instanceRegion reads the region segment of an instance OCID.
func instanceRegion(id string) (string, bool) {
	parts := strings.Split(id, ".")
	if len(parts) < 5 {
		return "", false
	}
	seg := parts[3]
	if oracleRegionRe.MatchString(seg) {
		return seg, true
	}
	r, ok := oracleRegionKeys[seg]
	return r, ok
}

// instanceRegions lists where an instance may live: its own region when the
// OCID names one, else every subscribed region so no lookup defaults to the
// wrong one.
func (o *Oracle) instanceRegions(ctx context.Context, id string) ([]string, error) {
	if r, ok := instanceRegion(id); ok {
		return []string{r}, nil
	}
	regs, err := o.ListRegions(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, r := range regs {
		out = append(out, r.ID)
	}
	if len(out) == 0 {
		return nil, errors.New("provision: oracle has no subscribed region to search")
	}
	return out, nil
}

// regionAllowed refuses a launch region the credential's tenancy is not
// subscribed to.
func (o *Oracle) regionAllowed(ctx context.Context, region string) error {
	regs, err := o.ListRegions(ctx)
	if err != nil {
		return err
	}
	for _, r := range regs {
		if r.ID == region {
			return nil
		}
	}
	return errors.New("provision: oracle region is not one this credential may use")
}

// HomeRegion returns the tenancy home region, where the free allowance lives.
func (o *Oracle) HomeRegion(ctx context.Context) (string, error) {
	var subs []struct {
		RegionName string `json:"regionName"`
		IsHome     bool   `json:"isHomeRegion"`
	}
	path := "/tenancies/" + url.PathEscape(o.cred.TenancyOCID) + "/regionSubscriptions"
	if err := o.identity(o.cred.Region).do(ctx, "GET", path, nil, &subs); err != nil {
		return "", err
	}
	for _, s := range subs {
		if s.IsHome && s.RegionName != "" {
			return s.RegionName, nil
		}
	}
	return "", errors.New("provision: oracle home region could not be determined")
}

// ListRegions returns the regions the tenancy is subscribed to.
func (o *Oracle) ListRegions(ctx context.Context) ([]Region, error) {
	var subs []struct {
		RegionName string `json:"regionName"`
		Status     string `json:"status"`
	}
	path := "/tenancies/" + url.PathEscape(o.cred.TenancyOCID) + "/regionSubscriptions"
	if err := o.identity(o.cred.Region).do(ctx, "GET", path, nil, &subs); err != nil {
		return nil, err
	}
	out := make([]Region, 0, len(subs))
	for _, s := range subs {
		if s.Status == "READY" || s.Status == "" {
			out = append(out, Region{ID: s.RegionName, Name: s.RegionName})
		}
	}
	return out, nil
}

type ociShape struct {
	Shape       string  `json:"shape"`
	Ocpus       float64 `json:"ocpus"`
	MemoryInGBs float64 `json:"memoryInGBs"`
	OcpuOptions *struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"ocpuOptions"`
	MemoryOptions *struct {
		MinInGBs float64 `json:"minInGBs"`
		MaxInGBs float64 `json:"maxInGBs"`
	} `json:"memoryOptions"`
}

func (s ociShape) flexible() bool { return s.OcpuOptions != nil && s.OcpuOptions.Max > 0 }

type flexPreset struct {
	ocpus  int
	memory int
}

func parseFlexPresets(v string) []flexPreset {
	var out []flexPreset
	for _, part := range strings.Split(v, ",") {
		o, m, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		oc, err1 := strconv.Atoi(o)
		mem, err2 := strconv.Atoi(m)
		if err1 != nil || err2 != nil || oc <= 0 || mem <= 0 {
			continue
		}
		out = append(out, flexPreset{oc, mem})
	}
	return out
}

func (s ociShape) allows(p flexPreset) bool {
	if !s.flexible() {
		return false
	}
	oc, mem := float64(p.ocpus), float64(p.memory)
	if oc < s.OcpuOptions.Min || oc > s.OcpuOptions.Max {
		return false
	}
	if s.MemoryOptions != nil && (mem < s.MemoryOptions.MinInGBs || mem > s.MemoryOptions.MaxInGBs) {
		return false
	}
	return true
}

// availabilityDomains returns the allowed ADs in API order.
func (o *Oracle) availabilityDomains(ctx context.Context, region string) ([]string, error) {
	var ads []struct {
		Name string `json:"name"`
	}
	path := "/availabilityDomains?compartmentId=" + url.QueryEscape(o.cred.TenancyOCID)
	if err := o.identity(region).do(ctx, "GET", path, nil, &ads); err != nil {
		return nil, err
	}
	var out []string
	for _, ad := range ads {
		if len(o.settings.AvailabilityDomains) == 0 || adAllowed(ad.Name, o.settings.AvailabilityDomains) {
			out = append(out, ad.Name)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("provision: oracle has no allowed availability domain in this region")
	}
	return out, nil
}

func adAllowed(name string, allowed []string) bool {
	for _, a := range allowed {
		if a != "" && strings.HasSuffix(name, a) {
			return true
		}
	}
	return false
}

func (o *Oracle) shapes(ctx context.Context, region, ad string) ([]ociShape, error) {
	var shapes []ociShape
	path := "/shapes?compartmentId=" + url.QueryEscape(o.cred.CompartmentOCID) + "&availabilityDomain=" + url.QueryEscape(ad)
	if err := o.iaas(region).do(ctx, "GET", path, nil, &shapes); err != nil {
		return nil, err
	}
	return shapes, nil
}

// ListSizes returns fixed shapes as listed and Flex shapes expanded into the
// configured presets, with the preset encoded in the size id. No price is
// returned because the compute API has none.
func (o *Oracle) ListSizes(ctx context.Context, region string) ([]Size, error) {
	region = o.regionOr(region)
	ads, err := o.availabilityDomains(ctx, region)
	if err != nil {
		return nil, err
	}
	shapes, err := o.shapes(ctx, region, ads[0])
	if err != nil {
		return nil, err
	}
	presets := parseFlexPresets(o.settings.FlexPresets)
	var out []Size
	seen := map[string]bool{}
	for _, s := range shapes {
		if s.flexible() {
			for _, p := range presets {
				if !s.allows(p) {
					continue
				}
				id := fmt.Sprintf("%s:%d:%d", s.Shape, p.ocpus, p.memory)
				out = append(out, Size{ID: id, Name: id, VCPUs: p.ocpus, Memory: p.memory * 1024, Disk: o.settings.BootVolumeGB})
			}
			continue
		}
		if seen[s.Shape] {
			continue
		}
		seen[s.Shape] = true
		out = append(out, Size{ID: s.Shape, Name: s.Shape, VCPUs: int(s.Ocpus), Memory: int(s.MemoryInGBs * 1024), Disk: o.settings.BootVolumeGB})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// parseOracleSize splits "Shape" or "Shape:ocpus:memoryGB".
func parseOracleSize(id string) (shape string, p flexPreset, flex bool, err error) {
	parts := strings.Split(id, ":")
	switch len(parts) {
	case 1:
		if strings.HasSuffix(parts[0], ".Flex") {
			return "", flexPreset{}, false, errors.New("provision: oracle flexible shapes need explicit ocpus and memory")
		}
		return parts[0], flexPreset{}, false, nil
	case 3:
		oc, e1 := strconv.Atoi(parts[1])
		mem, e2 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || oc <= 0 || mem <= 0 {
			return "", flexPreset{}, false, errors.New("provision: oracle size id is not valid")
		}
		return parts[0], flexPreset{oc, mem}, true, nil
	}
	return "", flexPreset{}, false, errors.New("provision: oracle size id is not valid")
}

func (o *Oracle) findImage(ctx context.Context, region, shape string) (string, error) {
	q := url.Values{}
	q.Set("compartmentId", o.cred.TenancyOCID)
	q.Set("operatingSystem", o.settings.ImageOS)
	q.Set("operatingSystemVersion", o.settings.ImageOSVersion)
	q.Set("shape", shape)
	q.Set("sortBy", "TIMECREATED")
	q.Set("sortOrder", "DESC")
	q.Set("limit", "1")
	var imgs []struct {
		ID string `json:"id"`
	}
	if err := o.iaas(region).do(ctx, "GET", "/images?"+q.Encode(), nil, &imgs); err != nil {
		return "", err
	}
	if len(imgs) == 0 || imgs[0].ID == "" {
		return "", errors.New("provision: oracle has no image for this shape")
	}
	return imgs[0].ID, nil
}

// isCapacityError matches only OCI's out-of-host-capacity answer.
func isCapacityError(err error) bool {
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != 500 {
		return false
	}
	return strings.Contains(strings.ToLower(pe.ociMsg), "out of host capacity")
}

// CreateServer launches one instance, trying each allowed availability domain
// once when OCI reports no host capacity. Any other error stops at once.
func (o *Oracle) CreateServer(ctx context.Context, opts CreateOpts) (string, string, error) {
	if o.cred.SubnetOCID == "" {
		return "", "", errors.New("provision: oracle needs a subnet OCID")
	}
	region := o.regionOr(opts.Region)
	shape, preset, flex, err := parseOracleSize(opts.Size)
	if err != nil {
		return "", "", err
	}
	if m := o.settings.MaxBootVolumeGB; m > 0 && o.settings.BootVolumeGB > m {
		return "", "", errors.New("provision: oracle boot volume is above provision.oracle.max_boot_volume_gb")
	}
	if err := o.regionAllowed(ctx, region); err != nil {
		return "", "", err
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(opts.UserData))
	if len(encoded) > maxOracleMetadataBytes {
		return "", "", errors.New("provision: oracle user data is larger than the metadata limit")
	}
	ads, err := o.availabilityDomains(ctx, region)
	if err != nil {
		return "", "", err
	}
	if flex {
		shapes, err := o.shapes(ctx, region, ads[0])
		if err != nil {
			return "", "", err
		}
		ok := false
		for _, s := range shapes {
			if s.Shape == shape && s.allows(preset) {
				ok = true
			}
		}
		if !ok {
			return "", "", errors.New("provision: oracle size is outside the shape limits")
		}
	}
	tagValue := opts.ProvisionID
	if tagValue == "" {
		tagValue = opts.Name
	}
	imageID, err := o.findImage(ctx, region, shape)
	if err != nil {
		return "", "", err
	}
	for i, ad := range ads {
		details := map[string]any{
			"availabilityDomain": ad,
			"compartmentId":      o.cred.CompartmentOCID,
			"shape":              shape,
			"displayName":        opts.Name,
			"sourceDetails": map[string]any{
				"sourceType":          "image",
				"imageId":             imageID,
				"bootVolumeSizeInGBs": o.settings.BootVolumeGB,
			},
			"createVnicDetails": map[string]any{
				"subnetId":       o.cred.SubnetOCID,
				"assignPublicIp": true,
			},
			"metadata":     map[string]string{"user_data": encoded},
			"freeformTags": map[string]string{o.settings.LabelKey: tagValue},
		}
		if flex {
			details["shapeConfig"] = map[string]any{"ocpus": preset.ocpus, "memoryInGBs": preset.memory}
		}
		var inst struct {
			ID string `json:"id"`
		}
		// One token per AD body: OCI rejects a token reused with a different body.
		var hdr map[string]string
		if opts.ProvisionID != "" {
			token := opts.ProvisionID
			if i > 0 {
				token = fmt.Sprintf("%s-ad%d", token, i)
			}
			hdr = map[string]string{"opc-retry-token": token}
		}
		_, err := o.iaas(region).doHeaders(ctx, "POST", "/instances", hdr, details, &inst)
		if err != nil && hdr != nil && ctx.Err() == nil && !isProviderErr(err) {
			// Transport error: the launch may have landed, so replay with the same token.
			_, err = o.iaas(region).doHeaders(ctx, "POST", "/instances", hdr, details, &inst)
		}
		if err == nil {
			return inst.ID, "", nil
		}
		if !isCapacityError(err) {
			return "", "", err
		}
	}
	return "", "", errOracleNoCapacity
}

func validOracleInstanceID(id string) bool {
	return strings.HasPrefix(id, "ocid1.instance.") && oracleOCIDRe.MatchString(id)
}

func isProviderErr(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe)
}

// GetServer maps the instance state and, once it exists, looks up the public
// IP through its primary VNIC. It reads the instance's own region.
func (o *Oracle) GetServer(ctx context.Context, id string) (ServerStatus, string, error) {
	if !validOracleInstanceID(id) {
		return ServerStatusError, "", errors.New("provision: oracle instance id is not valid")
	}
	regions, err := o.instanceRegions(ctx, id)
	if err != nil {
		return ServerStatusError, "", err
	}
	var lastErr error
	for _, region := range regions {
		c := o.iaas(region)
		var inst struct {
			State string `json:"lifecycleState"`
		}
		if err := c.do(ctx, "GET", "/instances/"+url.PathEscape(id), nil, &inst); err != nil {
			lastErr = err
			var pe *ProviderError
			if errors.As(err, &pe) && pe.Status == 404 {
				continue
			}
			return ServerStatusError, "", err
		}
		switch inst.State {
		case "RUNNING":
			return ServerStatusRunning, o.publicIP(ctx, c, id), nil
		case "PROVISIONING", "STARTING", "CREATING_IMAGE", "MOVING":
			return ServerStatusPending, "", nil
		default:
			return ServerStatusError, "", nil
		}
	}
	return ServerStatusError, "", lastErr
}
func (o *Oracle) publicIP(ctx context.Context, c *httpClient, instanceID string) string {
	var atts []struct {
		VnicID string `json:"vnicId"`
		State  string `json:"lifecycleState"`
	}
	path := "/vnicAttachments?compartmentId=" + url.QueryEscape(o.cred.CompartmentOCID) + "&instanceId=" + url.QueryEscape(instanceID)
	if err := c.do(ctx, "GET", path, nil, &atts); err != nil {
		return ""
	}
	for _, a := range atts {
		if a.VnicID == "" || (a.State != "" && a.State != "ATTACHED") {
			continue
		}
		var v struct {
			PublicIP  string `json:"publicIp"`
			IsPrimary bool   `json:"isPrimary"`
		}
		if err := c.do(ctx, "GET", "/vnics/"+url.PathEscape(a.VnicID), nil, &v); err == nil && v.PublicIP != "" {
			return v.PublicIP
		}
	}
	return ""
}

// DeleteServer terminates the instance and its boot volume in the instance's
// own region. A 404 counts as deleted only when the OCID names that region or
// every subscribed region answered 404.
func (o *Oracle) DeleteServer(ctx context.Context, id string) error {
	if !validOracleInstanceID(id) {
		return errors.New("provision: oracle instance id is not valid")
	}
	regions, err := o.instanceRegions(ctx, id)
	if err != nil {
		return err
	}
	for _, region := range regions {
		err := o.iaas(region).do(ctx, "DELETE", "/instances/"+url.PathEscape(id)+"?preserveBootVolume=false", nil, nil)
		var pe *ProviderError
		if errors.As(err, &pe) && pe.Status == 404 {
			continue
		}
		return err
	}
	return nil
}

// FindServerByTag returns a live instance carrying the provision row id tag,
// so a create that failed without an id cannot leak a server.
func (o *Oracle) FindServerByTag(ctx context.Context, region, tag string) (string, error) {
	region = o.regionOr(region)
	rows, err := o.listInstances(ctx, region, o.cred.CompartmentOCID)
	if err != nil {
		return "", err
	}
	for _, r := range rows {
		if r.FreeformTags[o.settings.LabelKey] == tag && r.State != "TERMINATED" && r.State != "TERMINATING" && validOracleInstanceID(r.ID) {
			return r.ID, nil
		}
	}
	return "", nil
}
