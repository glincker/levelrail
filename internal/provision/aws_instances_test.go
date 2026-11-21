package provision

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

type instFakeEC2 struct {
	insts      map[string]*types.Instance
	calls      []string
	stopSteps  int // describe calls before a stopping instance reads stopped
	pending    int
	modifyErr  error
	typeInfo   map[string]types.InstanceTypeInfo
	modifyType string
	delOnTerm  []bool
	pageTwo    bool
}

func (f *instFakeEC2) rec(s string) { f.calls = append(f.calls, s) }

func (f *instFakeEC2) DescribeInstances(_ context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	f.rec("describe")
	out := &ec2.DescribeInstancesOutput{}
	if len(in.InstanceIds) == 0 && !f.pageTwo {
		f.pageTwo = true
		for _, id := range []string{"i-managed"} {
			out.Reservations = append(out.Reservations, types.Reservation{Instances: []types.Instance{*f.insts[id]}})
		}
		out.NextToken = aws.String("p2")
		return out, nil
	}
	if len(in.InstanceIds) == 0 {
		for _, id := range []string{"i-foreign", "i-gone"} {
			out.Reservations = append(out.Reservations, types.Reservation{Instances: []types.Instance{*f.insts[id]}})
		}
		return out, nil
	}
	inst, ok := f.insts[in.InstanceIds[0]]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "InvalidInstanceID.NotFound"}
	}
	if inst.State.Name == types.InstanceStateNameStopping {
		f.pending++
		if f.pending > f.stopSteps {
			inst.State.Name = types.InstanceStateNameStopped
		}
	}
	out.Reservations = []types.Reservation{{Instances: []types.Instance{*inst}}}
	return out, nil
}

func (f *instFakeEC2) DescribeInstanceTypes(_ context.Context, in *ec2.DescribeInstanceTypesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	f.rec("types")
	out := &ec2.DescribeInstanceTypesOutput{}
	for name, info := range f.typeInfo {
		if len(in.InstanceTypes) == 0 {
			out.InstanceTypes = append(out.InstanceTypes, info)
			continue
		}
		for _, want := range in.InstanceTypes {
			if string(want) == name {
				out.InstanceTypes = append(out.InstanceTypes, info)
			}
		}
	}
	return out, nil
}

func (f *instFakeEC2) DescribeInstanceStatus(_ context.Context, _ *ec2.DescribeInstanceStatusInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceStatusOutput, error) {
	nb := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	return &ec2.DescribeInstanceStatusOutput{InstanceStatuses: []types.InstanceStatus{{Events: []types.InstanceStatusEvent{{Code: types.EventCodeInstanceReboot, NotBefore: &nb}}}}}, nil
}

func (f *instFakeEC2) DescribeVolumes(_ context.Context, _ *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return &ec2.DescribeVolumesOutput{Volumes: []types.Volume{{Size: aws.Int32(30)}}}, nil
}

func (f *instFakeEC2) StartInstances(_ context.Context, in *ec2.StartInstancesInput, _ ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	f.rec("start")
	f.insts[in.InstanceIds[0]].State.Name = types.InstanceStateNameRunning
	return &ec2.StartInstancesOutput{}, nil
}

func (f *instFakeEC2) StopInstances(_ context.Context, in *ec2.StopInstancesInput, _ ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	if aws.ToBool(in.Force) {
		f.rec("stop-force")
	} else {
		f.rec("stop")
	}
	f.insts[in.InstanceIds[0]].State.Name = types.InstanceStateNameStopping
	return &ec2.StopInstancesOutput{}, nil
}

func (f *instFakeEC2) RebootInstances(_ context.Context, _ *ec2.RebootInstancesInput, _ ...func(*ec2.Options)) (*ec2.RebootInstancesOutput, error) {
	f.rec("reboot")
	return &ec2.RebootInstancesOutput{}, nil
}

func (f *instFakeEC2) ModifyInstanceAttribute(_ context.Context, in *ec2.ModifyInstanceAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyInstanceAttributeOutput, error) {
	if in.InstanceType != nil {
		f.rec("modify-type")
		if f.modifyErr != nil {
			return nil, f.modifyErr
		}
		f.modifyType = aws.ToString(in.InstanceType.Value)
		return &ec2.ModifyInstanceAttributeOutput{}, nil
	}
	f.rec("modify-bdm")
	f.delOnTerm = append(f.delOnTerm, aws.ToBool(in.BlockDeviceMappings[0].Ebs.DeleteOnTermination))
	return &ec2.ModifyInstanceAttributeOutput{}, nil
}

func (f *instFakeEC2) CreateTags(_ context.Context, _ *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	f.rec("create-tags")
	return &ec2.CreateTagsOutput{}, nil
}

func (f *instFakeEC2) DeleteTags(_ context.Context, _ *ec2.DeleteTagsInput, _ ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error) {
	f.rec("delete-tags")
	return &ec2.DeleteTagsOutput{}, nil
}

func (f *instFakeEC2) TerminateInstances(_ context.Context, _ *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	f.rec("terminate")
	return &ec2.TerminateInstancesOutput{}, nil
}

type instFakeFactory struct{ c *instFakeEC2 }

func (f instFakeFactory) ForRegion(context.Context, string) (ec2InstanceAPI, error) { return f.c, nil }

func ownedInst(id string, state types.InstanceStateName, typ string, tags ...types.Tag) *types.Instance {
	return &types.Instance{
		InstanceId: aws.String(id), InstanceType: types.InstanceType(typ), State: &types.InstanceState{Name: state},
		Tags: tags, RootDeviceName: aws.String("/dev/xvda"),
		BlockDeviceMappings: []types.InstanceBlockDeviceMapping{{
			DeviceName: aws.String("/dev/xvda"),
			Ebs:        &types.EbsInstanceBlockDevice{VolumeId: aws.String("vol-1"), DeleteOnTermination: aws.Bool(true)},
		}},
	}
}

func newInstAWS() (*AWSInstances, *instFakeEC2) {
	managed := []types.Tag{{Key: aws.String("levelrail:managed"), Value: aws.String("true")}}
	f := &instFakeEC2{
		insts: map[string]*types.Instance{
			"i-managed": ownedInst("i-managed", types.InstanceStateNameRunning, "t3.small", managed...),
			"i-foreign": ownedInst("i-foreign", types.InstanceStateNameRunning, "t3.small"),
			"i-gone":    ownedInst("i-gone", types.InstanceStateNameTerminated, "t3.small", managed...),
		},
		typeInfo: map[string]types.InstanceTypeInfo{
			"t3.small": {InstanceType: "t3.small", VCpuInfo: &types.VCpuInfo{DefaultVCpus: aws.Int32(2)}, MemoryInfo: &types.MemoryInfo{SizeInMiB: aws.Int64(2048)}},
			"t3.large": {InstanceType: "t3.large", VCpuInfo: &types.VCpuInfo{DefaultVCpus: aws.Int32(2)}, MemoryInfo: &types.MemoryInfo{SizeInMiB: aws.Int64(8192)}},
		},
	}
	a := newAWSInstances(AWSCredentials{Region: "eu-west-1"}, instFakeFactory{f}, "levelrail", AWSInstanceSettings{})
	a.sleep = func(context.Context, time.Duration) error { return nil }
	return a, f
}

func TestAWSInstancesListOwnershipAndPaging(t *testing.T) {
	a, _ := newInstAWS()
	rows, err := a.ListInstances(context.Background(), InstanceScope{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %+v %v", rows, err)
	}
	byID := map[string]Instance{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	m, f := byID["eu-west-1/i-managed"], byID["eu-west-1/i-foreign"]
	if !m.Managed || m.ReadOnly || f.Managed || !f.ReadOnly {
		t.Fatalf("ownership %+v %+v", m, f)
	}
	if m.OCPUs != 2 || m.MemoryGB != 2 {
		t.Fatalf("spec %+v", m)
	}
}

func TestAWSInstancesActionMapping(t *testing.T) {
	cases := map[InstanceActionKind]string{
		InstanceActionStart: "start", InstanceActionStop: "stop-force",
		InstanceActionSoftStop: "stop", InstanceActionSoftReboot: "reboot",
	}
	for kind, want := range cases {
		a, f := newInstAWS()
		if err := a.InstanceAction(context.Background(), "eu-west-1/i-managed", kind); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if f.calls[len(f.calls)-1] != want {
			t.Fatalf("%s mapped to %v", kind, f.calls)
		}
	}
	a, _ := newInstAWS()
	if err := a.InstanceAction(context.Background(), "eu-west-1/i-managed", InstanceActionReboot); !errors.Is(err, ErrInstanceUnsupported) {
		t.Fatalf("hard reboot: %v", err)
	}
	if err := a.InstanceAction(context.Background(), "eu-west-1/i-foreign", InstanceActionStop); !errors.Is(err, ErrInstanceReadOnly) {
		t.Fatalf("unmanaged stop: %v", err)
	}
	if err := a.InstanceAction(context.Background(), "eu-west-1/i-nope", InstanceActionStart); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if InstanceCapabilities(a).SupportsAction(InstanceActionReboot) {
		t.Fatal("capability matrix must hide hard reboot")
	}
}

func TestAWSInstancesResizeStateMachine(t *testing.T) {
	a, f := newInstAWS()
	f.stopSteps = 2
	if err := a.ResizeInstance(context.Background(), "eu-west-1/i-managed", ResizeSpec{InstanceType: "t3.large"}); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, c := range f.calls {
		if c == "stop" || c == "modify-type" || c == "start" {
			order = append(order, c)
		}
	}
	if strings.Join(order, ",") != "stop,modify-type,start" || f.modifyType != "t3.large" {
		t.Fatalf("order %v type %q", order, f.modifyType)
	}

	// A stopped instance is changed and left stopped.
	a2, f2 := newInstAWS()
	f2.insts["i-managed"].State.Name = types.InstanceStateNameStopped
	if err := a2.ResizeInstance(context.Background(), "eu-west-1/i-managed", ResizeSpec{InstanceType: "t3.large"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range f2.calls {
		if c == "start" || c == "stop" {
			t.Fatalf("stopped instance must not be started or stopped: %v", f2.calls)
		}
	}

	// A type the API does not offer is refused before any stop.
	a3, f3 := newInstAWS()
	if err := a3.ResizeInstance(context.Background(), "eu-west-1/i-managed", ResizeSpec{InstanceType: "x9.huge"}); !errors.Is(err, ErrInstanceInvalid) {
		t.Fatalf("unknown type: %v", err)
	}
	for _, c := range f3.calls {
		if c == "stop" {
			t.Fatal("must not stop for an invalid type")
		}
	}

	// A failed modify after our own stop restarts the instance.
	a4, f4 := newInstAWS()
	f4.modifyErr = &smithy.GenericAPIError{Code: "IncorrectInstanceState"}
	err := a4.ResizeInstance(context.Background(), "eu-west-1/i-managed", ResizeSpec{InstanceType: "t3.large"})
	if !errors.Is(err, ErrInstanceConflict) || f4.calls[len(f4.calls)-1] != "start" {
		t.Fatalf("restore: %v %v", err, f4.calls)
	}
}

func TestAWSInstancesTagsAndTerminate(t *testing.T) {
	a, f := newInstAWS()
	ctx := context.Background()
	if err := a.SetInstanceTags(ctx, "eu-west-1/i-managed", map[string]string{"levelrail:managed": "false"}); !errors.Is(err, ErrInstanceInvalid) {
		t.Fatalf("reserved tag: %v", err)
	}
	if err := a.SetInstanceTags(ctx, "eu-west-1/i-managed", map[string]string{"team": "a", "old": ""}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetInstanceTags(ctx, "eu-west-1/i-foreign", map[string]string{"team": "a"}); !errors.Is(err, ErrInstanceReadOnly) {
		t.Fatalf("unmanaged tags: %v", err)
	}
	if err := a.TerminateInstance(ctx, "eu-west-1/i-managed", true); err != nil {
		t.Fatal(err)
	}
	if len(f.delOnTerm) != 1 || f.delOnTerm[0] || f.calls[len(f.calls)-1] != "terminate" {
		t.Fatalf("keep boot volume: %v %v", f.delOnTerm, f.calls)
	}
	a2, f2 := newInstAWS()
	if err := a2.TerminateInstance(ctx, "eu-west-1/i-managed", false); err != nil {
		t.Fatal(err)
	}
	if len(f2.delOnTerm) != 0 {
		t.Fatalf("default already deletes, no change expected: %v", f2.delOnTerm)
	}
	ev, err := a.InstanceEvents(ctx, "eu-west-1/i-managed", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(ev) != 1 || ev[0].Kind != "instance-reboot" {
		t.Fatalf("events %+v %v", ev, err)
	}
	sizes, err := a.ListInstanceTypes(ctx, "", "")
	if err != nil || len(sizes) != 2 {
		t.Fatalf("types %+v %v", sizes, err)
	}
}
