package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/glincker/levelrail/internal/awsx"
	"github.com/glincker/levelrail/internal/awsx/acm"
	"github.com/glincker/levelrail/internal/awsx/cloudfront"
	"github.com/glincker/levelrail/internal/awsx/cloudtrail"
	"github.com/glincker/levelrail/internal/awsx/cloudwatch"
	"github.com/glincker/levelrail/internal/awsx/ecr"
	"github.com/glincker/levelrail/internal/awsx/elasticache"
	"github.com/glincker/levelrail/internal/awsx/lightsail"
	"github.com/glincker/levelrail/internal/awsx/rds"
	"github.com/glincker/levelrail/internal/awsx/secrets"
	"github.com/glincker/levelrail/internal/awsx/ssm"
	"github.com/glincker/levelrail/internal/cloudconn"
	"github.com/glincker/levelrail/internal/cloudguard"
	"github.com/glincker/levelrail/internal/cloudspend"
	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// deepProbe is one read only provider call. Action names the IAM action it needs so a denied
// read can say exactly what to allow.
type deepProbe struct {
	ID, Title, Action string
	Run               func(ctx context.Context) error
}

func deepErrorCode(err error) string {
	if c := awsx.CodeOf(err); c != "" {
		return c
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		return api.ErrorCode()
	}
	return ""
}

func deepIsAuth(err error) bool {
	if awsx.ClassOf(err) == awsx.ClassAuth {
		return true
	}
	c := deepErrorCode(err)
	return strings.HasPrefix(c, "AccessDenied") || c == "UnauthorizedOperation" || c == "UnauthorizedAccess" || c == "AuthFailure"
}

func deepIsThrottle(err error) bool {
	return awsx.ClassOf(err) == awsx.ClassThrottle || strings.Contains(deepErrorCode(err), "Throttl")
}

// deepStepFor turns one probe result into a step. Provider messages never reach the text.
func deepStepFor(p deepProbe, err error) doctorStep {
	s := doctorStep{ID: p.ID, Title: p.Title}
	switch {
	case err == nil:
		s.Status, s.Detail = doctorOK, "The read succeeded."
	case errors.Is(err, context.DeadlineExceeded):
		s.Status, s.Detail = doctorFail, "The read timed out."
		s.Fix = "Check the host network and the region, or raise cloud.doctor.deep_timeout_seconds, then run the deep check again."
	case deepIsAuth(err):
		s.Status, s.Detail = doctorWarn, "The credential is not allowed to make this read."
		s.Fix = "Allow " + p.Action
		if tier, ok := cloudconn.CapabilityForAction(p.Action); ok {
			s.Fix += " (part of the " + tier + " tier in the link kit)"
		}
		s.Fix += " for the role, then run the deep check again."
	case deepIsThrottle(err):
		s.Status, s.Detail = doctorWarn, "The provider is throttling reads."
		s.Fix = "Wait a minute and run the deep check again."
	default:
		s.Status, s.Detail = doctorFail, "The read failed."
		if c := deepErrorCode(err); c != "" {
			s.Detail = "The read failed (" + c + ")."
		}
		s.Fix = "Check the provider status and the region, then run the deep check again."
	}
	return s
}

// runDeepProbes runs probes with bounded parallelism and a per step timeout, keeping order.
func runDeepProbes(ctx context.Context, probes []deepProbe, parallel int, timeout time.Duration) []doctorStep {
	if parallel < 1 {
		parallel = 1
	}
	steps := make([]doctorStep, len(probes))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- p.Run(sctx) }()
			select {
			case err := <-done:
				steps[i] = deepStepFor(p, err)
			case <-sctx.Done():
				steps[i] = deepStepFor(p, sctx.Err())
			}
		}()
	}
	wg.Wait()
	return steps
}

func (rt *Router) deepParallel() int {
	return rt.tunableInt(context.Background(), runtimesettings.KeyCloudDoctorDeepParallel, "APP_CLOUD_DOCTOR_DEEP_PARALLEL", 4)
}

func (rt *Router) deepTimeout() time.Duration {
	return time.Duration(rt.tunableInt(context.Background(), runtimesettings.KeyCloudDoctorDeepTimeoutSecs, "APP_CLOUD_DOCTOR_DEEP_TIMEOUT_SECONDS", 15)) * time.Second
}

var deepLastRun sync.Map

// deepAllowed spaces deep checks per account so a loop cannot hammer the provider.
func (rt *Router) deepAllowed(accountID string) bool {
	gap := time.Duration(rt.tunableInt(context.Background(), runtimesettings.KeyCloudDoctorDeepMinInterval, "APP_CLOUD_DOCTOR_DEEP_MIN_INTERVAL_SECONDS", 10)) * time.Second
	now := time.Now()
	if last, ok := deepLastRun.Load(accountID); ok && now.Sub(last.(time.Time)) < gap {
		return false
	}
	deepLastRun.Store(accountID, now)
	return true
}

func (rt *Router) deepAWSProbes(ctx context.Context, a store.CloudAccount) ([]deepProbe, []doctorStep, error) {
	creds, err := rt.accountAWSCredentials(ctx, a)
	if err != nil {
		return nil, nil, err
	}
	region := creds.Region
	if region == "" {
		region = awsDefaultRegion
	}
	p, err := creds.CredentialsProvider(ctx, region)
	if err != nil {
		return nil, nil, err
	}
	c, err := p.Retrieve(ctx)
	if err != nil {
		return nil, nil, err
	}
	cl, err := newAWSXClient(awsx.Credentials{AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey, SessionToken: c.SessionToken})
	if err != nil {
		return nil, nil, err
	}
	g := cloudguard.New(rt.cloudGuardConfig, nil)
	prefix := ""
	if rt.runtimeSettings != nil {
		prefix = strings.Trim(rt.runtimeSettings.String(ctx, runtimesettings.KeyCloudAWSNamePrefix), "/ ")
	}
	probes := []deepProbe{
		{"deep.aws.ec2", "EC2 instances", "ec2:DescribeInstances", func(ctx context.Context) error {
			_, err := ec2.New(ec2.Options{Region: region, Credentials: p}).DescribeInstances(ctx, &ec2.DescribeInstancesInput{MaxResults: aws.Int32(5)})
			return err
		}},
		{"deep.aws.regions", "Regions list", "ec2:DescribeRegions", func(ctx context.Context) error {
			_, err := ec2.New(ec2.Options{Region: region, Credentials: p}).DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
			return err
		}},
		{"deep.aws.s3", "S3 buckets", "s3:ListAllMyBuckets", func(ctx context.Context) error {
			_, err := s3.New(s3.Options{Region: region, Credentials: p}).ListBuckets(ctx, &s3.ListBucketsInput{MaxBuckets: aws.Int32(1)})
			return err
		}},
	}
	add := func(svc, id, title, action string, run func(context.Context) error) {
		if svc == "" || rt.awsServiceOn(svc) {
			probes = append(probes, deepProbe{id, title, action, run})
		}
	}
	add("ecr", "deep.aws.ecr", "ECR repositories", "ecr:DescribeRepositories", func(ctx context.Context) error {
		_, err := ecr.New(cl, g, ecr.Options{}).DescribeRepositories(ctx, region)
		return err
	})
	add("acm", "deep.aws.acm", "ACM certificates", "acm:ListCertificates", func(ctx context.Context) error {
		_, err := acm.New(cl, g).ListCertificates(ctx, region)
		return err
	})
	add("secrets", "deep.aws.secrets", "Secrets Manager names", "secretsmanager:ListSecrets", func(ctx context.Context) error {
		_, err := secrets.New(cl).ListSecrets(ctx, region)
		return err
	})
	add("secrets", "deep.aws.parameters", "Parameter Store names", "ssm:GetParametersByPath", func(ctx context.Context) error {
		_, err := secrets.New(cl).GetParametersByPath(ctx, region, "/"+prefix, false)
		return err
	})
	add("cloudtrail", "deep.aws.cloudtrail", "CloudTrail events", "cloudtrail:LookupEvents", func(ctx context.Context) error {
		_, err := cloudtrail.New(cl).LookupEvents(ctx, cloudtrail.Query{Region: region, MaxPages: 1})
		return err
	})
	add("cloudwatch", "deep.aws.cloudwatch", "CloudWatch metrics", "cloudwatch:ListMetrics", func(ctx context.Context) error {
		_, err := cloudwatch.New(cl).ListMetrics(ctx, region, "AWS/EC2", "", nil)
		return err
	})
	add("rds", "deep.aws.rds", "RDS databases", "rds:DescribeDBInstances", func(ctx context.Context) error {
		_, err := rds.New(cl, g).DescribeDBInstances(ctx, region)
		return err
	})
	add("elasticache", "deep.aws.elasticache", "ElastiCache clusters", "elasticache:DescribeCacheClusters", func(ctx context.Context) error {
		_, err := elasticache.New(cl, g).DescribeCacheClusters(ctx, region)
		return err
	})
	if rt.cloudSpendBool(runtimesettings.KeyCloudGuardCloudFront, "APP_CLOUD_GUARD_CLOUDFRONT", false) {
		add("", "deep.aws.cloudfront", "CloudFront distributions", "cloudfront:ListDistributions", func(ctx context.Context) error {
			_, err := cloudfront.New(cl).ListDistributions(ctx, "")
			return err
		})
	}
	add("lightsail", "deep.aws.lightsail", "Lightsail instances", "lightsail:GetInstances", func(ctx context.Context) error {
		_, err := lightsail.New(cl, g).GetInstances(ctx, region)
		return err
	})
	if rt.cloudSpendBool(runtimesettings.KeyCloudGuardSSM, "APP_CLOUD_GUARD_SSM", false) {
		add("", "deep.aws.ssm", "Systems Manager managed instances", "ssm:DescribeInstanceInformation", func(ctx context.Context) error {
			_, err := ssm.New(cl).DescribeInstances(ctx, region, "")
			return err
		})
	}
	var notes []doctorStep
	if rd, rerr := rt.cloudSpendReader(ctx, a); rerr == nil {
		q := cloudspend.SpendQuery{AccountID: a.ID}
		probes = append(probes, deepProbe{"deep.aws.budgets", "Budgets", "budgets:ViewBudget", func(ctx context.Context) error {
			_, err := rd.Budgets(ctx, q)
			return err
		}})
		if rt.doctorCostExplorerOn() {
			probes = append(probes, deepProbe{"deep.aws.cost_explorer", "Cost Explorer", "ce:GetCostAndUsage", func(ctx context.Context) error {
				_, err := rd.Spend(ctx, q)
				return err
			}})
		}
	}
	if !rt.doctorCostExplorerOn() {
		notes = append(notes, doctorStep{ID: "deep.aws.cost_explorer", Title: "Cost Explorer", Status: doctorSkip,
			Detail: "Skipped, AWS bills each Cost Explorer request.", Fix: "Turn on cloud.spend.aws_cost_explorer to include it."})
	}
	return probes, notes, nil
}

// deepOracleSteps reuses the capability probe the doctor already ran, so no second call is made.
func deepOracleSteps(res []cloudconn.CapabilityResult) []doctorStep {
	byName := map[string]cloudconn.CapabilityResult{}
	for _, r := range res {
		byName[r.Name] = r
	}
	titles := []struct{ name, title string }{
		{cloudconn.OracleCapCompute, "Compute list"}, {cloudconn.OracleCapUsage, "Usage read"},
		{cloudconn.OracleCapBudgets, "Budgets read"}, {cloudconn.OracleCapNetwork, "Network read (VNIC)"},
	}
	out := make([]doctorStep, 0, len(titles))
	for _, t := range titles {
		r := byName[t.name]
		s := doctorStep{ID: "deep.oracle." + t.name, Title: t.title}
		switch {
		case r.OK:
			s.Status, s.Detail = doctorOK, "The read succeeded."
		case r.Verified:
			s.Status, s.Detail = doctorWarn, "The key is not allowed to make this read."
			s.Fix = r.Hint + " The policy kit prints the statements (cloud oracle policy-kit)."
		default:
			s.Status, s.Detail = doctorFail, "The read could not complete."
			s.Fix = "Check the host network and the region, then run the deep check again."
		}
		out = append(out, s)
	}
	return out
}

// deepServerListProbe lists servers for the token based providers, one call.
func (rt *Router) deepServerListProbe(a store.CloudAccount) deepProbe {
	return deepProbe{"deep." + a.Provider + ".servers", "Server list", "read access on the API token", func(ctx context.Context) error {
		p, ok, _, _ := rt.resolveNodeProvisionerForAccount(ctx, a.Provider, a.ID)
		if !ok {
			return errors.New("no usable credential")
		}
		im, ok := p.(provision.InstanceManager)
		if !ok {
			return errors.New("no instance support")
		}
		_, err := im.ListInstances(ctx, provision.InstanceScope{})
		if err != nil {
			if status, _ := instanceErrorReply(err); status == http.StatusUnauthorized || status == http.StatusForbidden {
				return &awsx.Error{Class: awsx.ClassAuth, Code: "AccessDenied"}
			} else if status == http.StatusTooManyRequests {
				return &awsx.Error{Class: awsx.ClassThrottle, Code: "Throttling"}
			}
		}
		return err
	}}
}

// deepDoctorSteps builds the extra steps for an account; oracleRes is the capability probe already run.
func (rt *Router) deepDoctorSteps(ctx context.Context, a store.CloudAccount, oracleRes []cloudconn.CapabilityResult) []doctorStep {
	var probes []deepProbe
	var notes []doctorStep
	switch {
	case isAWSBrokeredAccount(a):
		var err error
		if probes, notes, err = rt.deepAWSProbes(ctx, a); err != nil {
			return []doctorStep{{ID: "deep.aws.credential", Title: "Deep check", Status: doctorFail,
				Detail: "The stored credential could not be used.", Fix: "Store the credential again with the link flow, then run the deep check again."}}
		}
	case isOracleAccount(a):
		return deepOracleSteps(oracleRes)
	case a.Provider == cloudconn.ProviderDigitalOcean || a.Provider == cloudconn.ProviderHetzner:
		probes = []deepProbe{rt.deepServerListProbe(a)}
	default:
		return nil
	}
	steps := runDeepProbes(ctx, probes, rt.deepParallel(), rt.deepTimeout())
	return append(steps, notes...)
}

// deepSummary counts outcomes for the summary line.
func deepSummary(steps []doctorStep) string {
	var ok, warn, fail int
	for _, s := range steps {
		switch s.Status {
		case doctorOK:
			ok++
		case doctorWarn:
			warn++
		case doctorFail:
			fail++
		}
	}
	return fmt.Sprintf("%d pass, %d warn, %d fail", ok, warn, fail)
}
