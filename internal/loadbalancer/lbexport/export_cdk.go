package lbexport

import (
	"fmt"
	"strings"
)

var cdkAlgorithms = map[string]string{
	"round_robin":                "ROUND_ROBIN",
	"least_outstanding_requests": "LEAST_OUTSTANDING_REQUESTS",
	"weighted_random":            "WEIGHTED_RANDOM",
}

func pascal(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func renderCDK(name string, in ExportInput) (string, []string) {
	a := buildALB(name, in)
	id := pascal(name)
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("import { Duration, Stack, StackProps } from 'aws-cdk-lib';\n")
	w("import * as ec2 from 'aws-cdk-lib/aws-ec2';\n")
	w("import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';\n")
	w("import { Construct } from 'constructs';\n\n")
	w("export interface %sLbProps extends StackProps {\n  vpc: ec2.IVpc;\n}\n\n", id)
	w("export class %sLbStack extends Stack {\n", id)
	w("  constructor(scope: Construct, id: string, props: %sLbProps) {\n    super(scope, id, props);\n\n", id)
	w("    const lb = new elbv2.ApplicationLoadBalancer(this, '%sLb', {\n      vpc: props.vpc,\n      internetFacing: true,\n", id)
	if a.IdleSecs > 0 {
		w("      idleTimeout: Duration.seconds(%d),\n", a.IdleSecs)
	}
	w("    });\n\n")
	w("    const targetGroup = new elbv2.ApplicationTargetGroup(this, '%sTargets', {\n", id)
	w("      vpc: props.vpc,\n      port: %d,\n      protocol: elbv2.ApplicationProtocol.%s,\n      targetType: elbv2.TargetType.IP,\n", a.Port, a.Protocol)
	w("      loadBalancingAlgorithmType: elbv2.TargetGroupLoadBalancingAlgorithmType.%s,\n", cdkAlgorithms[a.Algorithm])
	if a.DrainSecs >= 0 {
		w("      deregistrationDelay: Duration.seconds(%d),\n", a.DrainSecs)
	}
	if a.SlowSecs > 0 {
		w("      slowStart: Duration.seconds(%d),\n", a.SlowSecs)
	}
	if a.Sticky {
		w("      stickinessCookieDuration: Duration.seconds(%d),\n", a.StickySecs)
	}
	w("      healthCheck: {\n        path: '%s',\n        interval: Duration.seconds(%d),\n        timeout: Duration.seconds(%d),\n", a.HealthPath, a.Interval, a.Timeout)
	w("        healthyThresholdCount: %d,\n        unhealthyThresholdCount: %d,\n        healthyHttpCodes: '%s',\n      },\n    });\n\n", a.Healthy, a.Unhealthy, a.Matcher)
	w("    lb.addListener('%sHttp', {\n      port: 80,\n      defaultAction: elbv2.ListenerAction.forward([targetGroup]),\n    });\n  }\n}\n", id)
	return b.String(), a.Warnings
}
