package provision

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// Tag suffixes under the brand-derived prefix. The prefix is the short name,
// so source never carries the product name.
const (
	awsTagManaged = "managed"
	awsTagProject = "project"
	awsTagEnv     = "env"
)

const (
	awsDefaultPollInterval = 5 * time.Second
	awsDefaultWaitTimeout  = 5 * time.Minute
)

// ec2InstanceAPI is the slice of the EC2 client instance management calls.
type ec2InstanceAPI interface {
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeInstanceTypes(ctx context.Context, in *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeInstanceStatus(ctx context.Context, in *ec2.DescribeInstanceStatusInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceStatusOutput, error)
	DescribeVolumes(ctx context.Context, in *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	StartInstances(ctx context.Context, in *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, in *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
	RebootInstances(ctx context.Context, in *ec2.RebootInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RebootInstancesOutput, error)
	ModifyInstanceAttribute(ctx context.Context, in *ec2.ModifyInstanceAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyInstanceAttributeOutput, error)
	CreateTags(ctx context.Context, in *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DeleteTags(ctx context.Context, in *ec2.DeleteTagsInput, optFns ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error)
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

// ec2ClientFactory builds a region-scoped client, so tests inject a fake.
type ec2ClientFactory interface {
	ForRegion(ctx context.Context, region string) (ec2InstanceAPI, error)
}

type awsClientFactory struct{ creds AWSCredentials }

func (f awsClientFactory) ForRegion(ctx context.Context, region string) (ec2InstanceAPI, error) {
	p, err := f.creds.CredentialsProvider(ctx, region)
	if err != nil {
		return nil, err
	}
	return ec2.New(ec2.Options{Region: region, Credentials: p}), nil
}

// AWSInstanceSettings are the tunables a settings slice can feed live.
type AWSInstanceSettings struct {
	// TagPrefix namespaces the ownership tags, derived from the brand short name when empty.
	TagPrefix    string
	PollInterval time.Duration
	WaitTimeout  time.Duration
	// AllowedRegions limits every call; empty means any region.
	AllowedRegions []string
}

// AWSInstances implements InstanceManager over EC2.
type AWSInstances struct {
	creds    AWSCredentials
	factory  ec2ClientFactory
	legacy   string
	prefix   string
	interval time.Duration
	timeout  time.Duration
	sleep    func(context.Context, time.Duration) error
	allowed  []string
	rawBase  string
}

var _ InstanceManager = (*AWSInstances)(nil)

// NewAWSInstances builds the manager; the region comes from the account config.
func NewAWSInstances(creds AWSCredentials, shortName string, s AWSInstanceSettings) *AWSInstances {
	return newAWSInstances(creds, awsClientFactory{creds: creds}, shortName, s)
}

func newAWSInstances(creds AWSCredentials, f ec2ClientFactory, shortName string, s AWSInstanceSettings) *AWSInstances {
	if s.TagPrefix == "" {
		s.TagPrefix = awsSanitizeGroupNameComponent(strings.ToLower(awsShortNameOrFallback(shortName))) + ":"
	}
	if s.PollInterval <= 0 {
		s.PollInterval = awsDefaultPollInterval
	}
	if s.WaitTimeout <= 0 {
		s.WaitTimeout = awsDefaultWaitTimeout
	}
	return &AWSInstances{
		creds: creds, factory: f, legacy: awsShortNameOrFallback(shortName), prefix: s.TagPrefix,
		interval: s.PollInterval, timeout: s.WaitTimeout, sleep: contextSleep, allowed: s.AllowedRegions,
	}
}

// InstanceCapabilities lists what EC2 can do. EC2 has no hard reboot, and its
// stop is graceful by default, so stop forces and soft_stop does not.
func (a *AWSInstances) InstanceCapabilities() CapabilityMatrix {
	return NewCapabilityMatrix(VerbList, VerbDescribe, VerbStart, VerbStop, VerbSoftStop,
		VerbSoftReboot, VerbResize, VerbResizeByShape, VerbTags, VerbTerminate, VerbKeepBootVol, VerbEvents)
}

func classifyAWS(err error) error {
	var ae smithy.APIError
	if err == nil || !errors.As(err, &ae) {
		return err
	}
	code := ae.ErrorCode()
	switch {
	case code == "IncorrectInstanceState" || code == "IncorrectState":
		return classify(ErrInstanceConflict, code, err)
	case strings.HasPrefix(code, "InvalidInstanceID.NotFound"):
		return classify(ErrInstanceNotFound, code, err)
	case code == "UnauthorizedOperation" || code == "AuthFailure" || code == "OptInRequired":
		return classify(ErrInstanceForbidden, code, err)
	case code == "RequestLimitExceeded" || code == "Throttling" || code == "ThrottlingException":
		return classify(ErrInstanceThrottled, code, err)
	case code == "InsufficientInstanceCapacity" || code == "InstanceLimitExceeded":
		return classify(ErrInstanceCapacity, code, err)
	case code == "UnsupportedOperation":
		return classify(ErrInstanceUnsupported, code, err)
	case strings.HasPrefix(code, "InvalidParameter") || strings.HasPrefix(code, "InvalidInstanceID") || code == "InvalidInstanceType" || code == "MissingParameter":
		return classify(ErrInstanceInvalid, code, err)
	}
	return err
}

func awsState(s *types.InstanceState) InstanceState {
	if s == nil {
		return InstanceStateUnknown
	}
	switch s.Name {
	case types.InstanceStateNamePending:
		return InstanceStatePending
	case types.InstanceStateNameRunning:
		return InstanceStateRunning
	case types.InstanceStateNameStopping:
		return InstanceStateStopping
	case types.InstanceStateNameStopped:
		return InstanceStateStopped
	case types.InstanceStateNameShuttingDown:
		return InstanceStateTerminating
	case types.InstanceStateNameTerminated:
		return InstanceStateTerminated
	}
	return InstanceStateUnknown
}

func (a *AWSInstances) managed(tags map[string]string) bool {
	if tags[a.prefix+awsTagManaged] == "true" {
		return true
	}
	return tags[awsManagedTagKey] == a.legacy
}

func (a *AWSInstances) toInstance(region string, in types.Instance) Instance {
	tags := map[string]string{}
	for _, t := range in.Tags {
		tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	m := a.managed(tags)
	out := Instance{
		ID: encodeAWSServerID(region, aws.ToString(in.InstanceId)), Name: tags["Name"], Region: region,
		State: awsState(in.State), Shape: string(in.InstanceType),
		PublicIPv4: aws.ToString(in.PublicIpAddress), Tags: tags, Managed: m, ReadOnly: !m,
	}
	if in.State != nil {
		out.RawState = string(in.State.Name)
	}
	if in.LaunchTime != nil {
		out.CreatedAt = in.LaunchTime.UTC()
	}
	return out
}

// typeSpecs fills vCPU and memory for the given types from DescribeInstanceTypes.
// A failure leaves the numbers empty rather than failing the listing.
func typeSpecs(ctx context.Context, c ec2InstanceAPI, names []string) map[string]types.InstanceTypeInfo {
	out := map[string]types.InstanceTypeInfo{}
	if len(names) == 0 {
		return out
	}
	in := &ec2.DescribeInstanceTypesInput{}
	for _, n := range names {
		in.InstanceTypes = append(in.InstanceTypes, types.InstanceType(n))
	}
	for pages := 0; pages < maxListPages; pages++ {
		res, err := c.DescribeInstanceTypes(ctx, in)
		if err != nil {
			return out
		}
		for _, t := range res.InstanceTypes {
			out[string(t.InstanceType)] = t
		}
		if res.NextToken == nil || *res.NextToken == "" {
			break
		}
		in.NextToken = res.NextToken
	}
	return out
}

func applySpec(i *Instance, spec map[string]types.InstanceTypeInfo) {
	t, ok := spec[i.Shape]
	if !ok {
		return
	}
	if t.VCpuInfo != nil {
		i.OCPUs = float64(aws.ToInt32(t.VCpuInfo.DefaultVCpus))
	}
	if t.MemoryInfo != nil {
		i.MemoryGB = float64(aws.ToInt64(t.MemoryInfo.SizeInMiB)) / 1024
	}
}

func (a *AWSInstances) client(ctx context.Context, region string) (ec2InstanceAPI, error) {
	if region == "" {
		region = a.creds.region()
	}
	if len(a.allowed) > 0 && !slices.Contains(a.allowed, region) {
		return nil, classify(ErrInstanceNotFound, "region", errors.New("provision: aws region is not allowed for this account"))
	}
	c, err := a.factory.ForRegion(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("provision: aws client for %s: %w", region, classifyAWS(err))
	}
	return c, nil
}

// ListInstances pages through DescribeInstances, hiding terminated instances.
// Instances without the ownership tags are returned read only.
func (a *AWSInstances) ListInstances(ctx context.Context, scope InstanceScope) ([]Instance, error) {
	region := scope.Region
	if region == "" {
		region = a.creds.region()
	}
	c, err := a.client(ctx, region)
	if err != nil {
		return nil, err
	}
	in := &ec2.DescribeInstancesInput{Filters: []types.Filter{{
		Name: aws.String("instance-state-name"),
		Values: []string{string(types.InstanceStateNamePending), string(types.InstanceStateNameRunning),
			string(types.InstanceStateNameStopping), string(types.InstanceStateNameStopped)},
	}}}
	out := []Instance{}
	for pages := 0; pages < maxListPages; pages++ {
		res, err := c.DescribeInstances(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("provision: aws list instances: %w", classifyAWS(err))
		}
		for _, r := range res.Reservations {
			for _, inst := range r.Instances {
				if s := awsState(inst.State); s == InstanceStateTerminated || s == InstanceStateTerminating {
					continue
				}
				out = append(out, a.toInstance(region, inst))
			}
		}
		if res.NextToken == nil || *res.NextToken == "" {
			break
		}
		in.NextToken = res.NextToken
	}
	names := []string{}
	seen := map[string]bool{}
	for _, i := range out {
		if !seen[i.Shape] {
			seen[i.Shape] = true
			names = append(names, i.Shape)
		}
	}
	spec := typeSpecs(ctx, c, names)
	for i := range out {
		applySpec(&out[i], spec)
	}
	return out, nil
}

// fetch returns the raw instance for a region/instance id.
func (a *AWSInstances) fetch(ctx context.Context, id string) (ec2InstanceAPI, string, string, types.Instance, error) {
	region, iid, err := decodeAWSServerID(id)
	if err != nil {
		return nil, "", "", types.Instance{}, classify(ErrInstanceInvalid, "id", err)
	}
	c, err := a.client(ctx, region)
	if err != nil {
		return nil, "", "", types.Instance{}, err
	}
	res, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{iid}})
	if err != nil {
		return nil, "", "", types.Instance{}, fmt.Errorf("provision: aws describe instance: %w", classifyAWS(err))
	}
	for _, r := range res.Reservations {
		for _, inst := range r.Instances {
			return c, region, iid, inst, nil
		}
	}
	return nil, "", "", types.Instance{}, classify(ErrInstanceNotFound, "", errors.New("provision: aws instance not found"))
}

// writable fetches the instance and refuses one this product does not manage.
func (a *AWSInstances) writable(ctx context.Context, id string) (ec2InstanceAPI, string, types.Instance, error) {
	c, region, iid, inst, err := a.fetch(ctx, id)
	if err != nil {
		return nil, "", types.Instance{}, err
	}
	if a.toInstance(region, inst).ReadOnly {
		return nil, "", types.Instance{}, classify(ErrInstanceReadOnly, "", errors.New("provision: aws instance is unmanaged"))
	}
	return c, iid, inst, nil
}

// DescribeInstance reads the instance with its type specs and boot volume size.
func (a *AWSInstances) DescribeInstance(ctx context.Context, id string) (InstanceDetail, error) {
	c, region, _, inst, err := a.fetch(ctx, id)
	if err != nil {
		return InstanceDetail{}, err
	}
	base := a.toInstance(region, inst)
	applySpec(&base, typeSpecs(ctx, c, []string{base.Shape}))
	d := InstanceDetail{
		Instance: base, PrivateIPv4: aws.ToString(inst.PrivateIpAddress), ImageID: aws.ToString(inst.ImageId),
		Project: base.Tags[a.prefix+awsTagProject], Environment: base.Tags[a.prefix+awsTagEnv],
		ResizeAdvisory: "resizing stops the instance, changes its type, then starts it again",
	}
	if inst.Placement != nil {
		d.Zone = aws.ToString(inst.Placement.AvailabilityZone)
	}
	if v := rootVolumeID(inst); v != "" {
		if res, err := c.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: []string{v}}); err == nil && len(res.Volumes) > 0 {
			d.BootVolumeGB = float64(aws.ToInt32(res.Volumes[0].Size))
		}
	}
	return d, nil
}

func rootVolumeID(inst types.Instance) string {
	for _, m := range inst.BlockDeviceMappings {
		if aws.ToString(m.DeviceName) == aws.ToString(inst.RootDeviceName) && m.Ebs != nil {
			return aws.ToString(m.Ebs.VolumeId)
		}
	}
	return ""
}

// InstanceAction maps a power action to EC2. Stop forces, soft_stop is the
// graceful default, soft_reboot is RebootInstances; hard reboot is unsupported.
func (a *AWSInstances) InstanceAction(ctx context.Context, id string, kind InstanceActionKind) error {
	if !a.InstanceCapabilities().SupportsAction(kind) {
		return classify(ErrInstanceUnsupported, string(kind), errors.New("provision: aws has no such action"))
	}
	c, iid, _, err := a.writable(ctx, id)
	if err != nil {
		return err
	}
	ids := []string{iid}
	switch kind {
	case InstanceActionStart:
		_, err = c.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: ids})
	case InstanceActionStop:
		_, err = c.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: ids, Force: aws.Bool(true)})
	case InstanceActionSoftStop:
		_, err = c.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: ids})
	case InstanceActionSoftReboot:
		_, err = c.RebootInstances(ctx, &ec2.RebootInstancesInput{InstanceIds: ids})
	}
	if err != nil {
		return fmt.Errorf("provision: aws %s: %w", kind, classifyAWS(err))
	}
	return nil
}

// ListInstanceTypes returns the types the API offers in a region, optionally
// limited to one CPU architecture. Nothing is hardcoded.
func (a *AWSInstances) ListInstanceTypes(ctx context.Context, region, arch string) ([]Size, error) {
	c, err := a.client(ctx, region)
	if err != nil {
		return nil, err
	}
	in := &ec2.DescribeInstanceTypesInput{}
	if arch != "" {
		in.Filters = []types.Filter{{Name: aws.String("processor-info.supported-architecture"), Values: []string{arch}}}
	}
	out := []Size{}
	for pages := 0; pages < maxListPages; pages++ {
		res, err := c.DescribeInstanceTypes(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("provision: aws list instance types: %w", classifyAWS(err))
		}
		for _, t := range res.InstanceTypes {
			s := Size{ID: string(t.InstanceType), Name: string(t.InstanceType)}
			if t.VCpuInfo != nil {
				s.VCPUs = int(aws.ToInt32(t.VCpuInfo.DefaultVCpus))
			}
			if t.MemoryInfo != nil {
				s.Memory = int(aws.ToInt64(t.MemoryInfo.SizeInMiB))
			}
			out = append(out, s)
		}
		if res.NextToken == nil || *res.NextToken == "" {
			break
		}
		in.NextToken = res.NextToken
	}
	return out, nil
}

// waitState polls until the instance reaches want or the wait timeout passes.
func (a *AWSInstances) waitState(ctx context.Context, c ec2InstanceAPI, iid string, want types.InstanceStateName) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	for {
		res, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{iid}})
		if err != nil {
			return fmt.Errorf("provision: aws wait for %s: %w", want, classifyAWS(err))
		}
		for _, r := range res.Reservations {
			for _, inst := range r.Instances {
				if inst.State != nil && inst.State.Name == want {
					return nil
				}
			}
		}
		if err := a.sleep(ctx, a.interval); err != nil {
			return fmt.Errorf("provision: aws instance did not reach %s in time: %w", want, err)
		}
	}
}

type awsResizeStep int

const (
	resizeStepStop awsResizeStep = iota
	resizeStepWaitStopped
	resizeStepModify
	resizeStepStart
	resizeStepDone
)

// ResizeInstance changes the instance type. Steps: validate, stop if running,
// wait stopped, modify, start only if it was running. If the modify fails after
// our own stop, a start is attempted so the instance is not left down.
func (a *AWSInstances) ResizeInstance(ctx context.Context, id string, spec ResizeSpec) error {
	if spec.InstanceType == "" {
		return classify(ErrInstanceInvalid, "instance_type", errors.New("provision: aws resize needs an instance type"))
	}
	c, iid, inst, err := a.writable(ctx, id)
	if err != nil {
		return err
	}
	if string(inst.InstanceType) == spec.InstanceType {
		return nil
	}
	res, err := c.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{InstanceTypes: []types.InstanceType{types.InstanceType(spec.InstanceType)}})
	if err != nil {
		return fmt.Errorf("provision: aws resize target: %w", classifyAWS(err))
	}
	if len(res.InstanceTypes) == 0 {
		return classify(ErrInstanceInvalid, "InvalidInstanceType", errors.New("provision: aws instance type not offered"))
	}
	state := awsState(inst.State)
	if state != InstanceStateRunning && state != InstanceStateStopped {
		return classify(ErrInstanceConflict, string(state), errors.New("provision: aws instance is not running or stopped"))
	}
	wasRunning := state == InstanceStateRunning
	step := resizeStepModify
	if wasRunning {
		step = resizeStepStop
	}
	for step != resizeStepDone {
		switch step {
		case resizeStepStop:
			if _, err := c.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{iid}}); err != nil {
				return fmt.Errorf("provision: aws resize stop: %w", classifyAWS(err))
			}
			step = resizeStepWaitStopped
		case resizeStepWaitStopped:
			if err := a.waitState(ctx, c, iid, types.InstanceStateNameStopped); err != nil {
				return err
			}
			step = resizeStepModify
		case resizeStepModify:
			_, err := c.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
				InstanceId:   aws.String(iid),
				InstanceType: &types.AttributeValue{Value: aws.String(spec.InstanceType)},
			})
			if err != nil {
				if wasRunning {
					_, _ = c.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{iid}})
				}
				return fmt.Errorf("provision: aws resize modify: %w", classifyAWS(err))
			}
			step = resizeStepDone
			if wasRunning {
				step = resizeStepStart
			}
		case resizeStepStart:
			if _, err := c.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{iid}}); err != nil {
				return fmt.Errorf("provision: aws resize start: %w", classifyAWS(err))
			}
			step = resizeStepDone
		}
	}
	return nil
}

// SetInstanceTags upserts tags and deletes keys given an empty value. The
// ownership and provider-reserved keys are refused so a user cannot reclaim or
// release an instance through this call.
func (a *AWSInstances) SetInstanceTags(ctx context.Context, id string, tags map[string]string) error {
	for k := range tags {
		if strings.HasPrefix(k, a.prefix) || strings.HasPrefix(strings.ToLower(k), "aws:") || k == awsManagedTagKey {
			return classify(ErrInstanceInvalid, "reserved_tag", errors.New("provision: aws tag key is reserved"))
		}
	}
	c, iid, _, err := a.writable(ctx, id)
	if err != nil {
		return err
	}
	var set, del []types.Tag
	for k, v := range tags {
		if v == "" {
			del = append(del, types.Tag{Key: aws.String(k)})
			continue
		}
		set = append(set, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	if len(set) > 0 {
		if _, err := c.CreateTags(ctx, &ec2.CreateTagsInput{Resources: []string{iid}, Tags: set}); err != nil {
			return fmt.Errorf("provision: aws create tags: %w", classifyAWS(err))
		}
	}
	if len(del) > 0 {
		if _, err := c.DeleteTags(ctx, &ec2.DeleteTagsInput{Resources: []string{iid}, Tags: del}); err != nil {
			return fmt.Errorf("provision: aws delete tags: %w", classifyAWS(err))
		}
	}
	return nil
}

// TerminateInstance sets the root volume's DeleteOnTermination to match
// keepBootVolume when it differs, then terminates.
func (a *AWSInstances) TerminateInstance(ctx context.Context, id string, keepBootVolume bool) error {
	c, iid, inst, err := a.writable(ctx, id)
	if err != nil {
		return err
	}
	for _, m := range inst.BlockDeviceMappings {
		if aws.ToString(m.DeviceName) != aws.ToString(inst.RootDeviceName) || m.Ebs == nil {
			continue
		}
		if aws.ToBool(m.Ebs.DeleteOnTermination) == !keepBootVolume {
			break
		}
		_, err := c.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
			InstanceId: aws.String(iid),
			BlockDeviceMappings: []types.InstanceBlockDeviceMappingSpecification{{
				DeviceName: m.DeviceName,
				Ebs:        &types.EbsInstanceBlockDeviceSpecification{DeleteOnTermination: aws.Bool(!keepBootVolume)},
			}},
		})
		if err != nil {
			return fmt.Errorf("provision: aws set boot volume retention: %w", classifyAWS(err))
		}
		break
	}
	if _, err := c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{iid}}); err != nil {
		return fmt.Errorf("provision: aws terminate instance: %w", classifyAWS(err))
	}
	return nil
}

// InstanceEvents returns scheduled events from DescribeInstanceStatus.
func (a *AWSInstances) InstanceEvents(ctx context.Context, id string, since time.Time) ([]InstanceEvent, error) {
	c, region, iid, _, err := a.fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	res, err := c.DescribeInstanceStatus(ctx, &ec2.DescribeInstanceStatusInput{InstanceIds: []string{iid}, IncludeAllInstances: aws.Bool(true)})
	if err != nil {
		return nil, fmt.Errorf("provision: aws instance events: %w", classifyAWS(err))
	}
	out := []InstanceEvent{}
	for _, s := range res.InstanceStatuses {
		for _, e := range s.Events {
			t := aws.ToTime(e.NotBefore)
			if t.Before(since) {
				continue
			}
			out = append(out, InstanceEvent{Time: t, Kind: string(e.Code), Status: "scheduled", Message: aws.ToString(e.Description)})
		}
	}
	// CloudTrail is best effort: a credential without it still gets scheduled events.
	if trail, terr := a.cloudTrailEvents(ctx, region, iid, since); terr == nil {
		out = append(out, trail...)
	}
	return out, nil
}
