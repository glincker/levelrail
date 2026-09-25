package lbexport

import (
	"fmt"
	"strings"
)

func renderCloudFormation(name string, in ExportInput) (string, []string) {
	a := buildALB(name, in)
	id := pascal(name)
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("AWSTemplateFormatVersion: '2010-09-09'\nDescription: Application load balancer for %s\n\n", name)
	w("Parameters:\n  VpcId:\n    Type: AWS::EC2::VPC::Id\n  SubnetIds:\n    Type: List<AWS::EC2::Subnet::Id>\n  SecurityGroupIds:\n    Type: List<AWS::EC2::SecurityGroup::Id>\n\n")
	w("Resources:\n")
	w("  %sLoadBalancer:\n    Type: AWS::ElasticLoadBalancingV2::LoadBalancer\n    Properties:\n      Name: %s\n      Type: application\n      Subnets: !Ref SubnetIds\n      SecurityGroups: !Ref SecurityGroupIds\n", id, a.TGName)
	if a.IdleSecs > 0 {
		w("      LoadBalancerAttributes:\n        - Key: idle_timeout.timeout_seconds\n          Value: '%d'\n", a.IdleSecs)
	}
	w("\n  %sTargetGroup:\n    Type: AWS::ElasticLoadBalancingV2::TargetGroup\n    Properties:\n", id)
	w("      Name: %s\n      Port: %d\n      Protocol: %s\n      TargetType: ip\n      VpcId: !Ref VpcId\n", a.TGName, a.Port, a.Protocol)
	w("      HealthCheckPath: %s\n      HealthCheckIntervalSeconds: %d\n      HealthCheckTimeoutSeconds: %d\n", a.HealthPath, a.Interval, a.Timeout)
	w("      HealthyThresholdCount: %d\n      UnhealthyThresholdCount: %d\n      Matcher:\n        HttpCode: '%s'\n", a.Healthy, a.Unhealthy, a.Matcher)
	w("      TargetGroupAttributes:\n")
	w("        - Key: load_balancing.algorithm.type\n          Value: %s\n", a.Algorithm)
	if a.DrainSecs >= 0 {
		w("        - Key: deregistration_delay.timeout_seconds\n          Value: '%d'\n", a.DrainSecs)
	}
	if a.SlowSecs > 0 {
		w("        - Key: slow_start.duration_seconds\n          Value: '%d'\n", a.SlowSecs)
	}
	if a.Sticky {
		w("        - Key: stickiness.enabled\n          Value: 'true'\n")
		w("        - Key: stickiness.type\n          Value: lb_cookie\n")
		w("        - Key: stickiness.lb_cookie.duration_seconds\n          Value: '%d'\n", a.StickySecs)
	}
	w("\n  %sListener:\n    Type: AWS::ElasticLoadBalancingV2::Listener\n    Properties:\n      LoadBalancerArn: !Ref %sLoadBalancer\n      Port: 80\n      Protocol: HTTP\n", id, id)
	w("      DefaultActions:\n        - Type: forward\n          TargetGroupArn: !Ref %sTargetGroup\n\n", id)
	w("Outputs:\n  LoadBalancerDNS:\n    Value: !GetAtt %sLoadBalancer.DNSName\n", id)
	return b.String(), a.Warnings
}
