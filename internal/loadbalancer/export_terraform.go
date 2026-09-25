package loadbalancer

import (
	"fmt"
	"strings"
)

func renderTerraform(name string, in ExportInput) (string, []string) {
	a := buildALB(name, in)
	res := strings.ReplaceAll(name, "-", "_")
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("variable \"vpc_id\" {\n  type = string\n}\n\n")
	w("variable \"subnet_ids\" {\n  type = list(string)\n}\n\n")
	w("variable \"security_group_ids\" {\n  type    = list(string)\n  default = []\n}\n\n")
	w("resource \"aws_lb\" \"%s\" {\n", res)
	w("  name               = \"%s\"\n  load_balancer_type = \"application\"\n", a.TGName)
	w("  subnets            = var.subnet_ids\n  security_groups    = var.security_group_ids\n")
	if a.IdleSecs > 0 {
		w("  idle_timeout       = %d\n", a.IdleSecs)
	}
	w("}\n\n")
	w("resource \"aws_lb_target_group\" \"%s\" {\n", res)
	w("  name                           = \"%s\"\n  port                           = %d\n", a.TGName, a.Port)
	w("  protocol                       = \"%s\"\n  target_type                    = \"ip\"\n  vpc_id                         = var.vpc_id\n", a.Protocol)
	w("  load_balancing_algorithm_type  = \"%s\"\n", a.Algorithm)
	if a.DrainSecs >= 0 {
		w("  deregistration_delay           = %d\n", a.DrainSecs)
	}
	if a.SlowSecs > 0 {
		w("  slow_start                     = %d\n", a.SlowSecs)
	}
	w("\n  health_check {\n    path                = \"%s\"\n    interval            = %d\n    timeout             = %d\n", a.HealthPath, a.Interval, a.Timeout)
	w("    healthy_threshold   = %d\n    unhealthy_threshold = %d\n    matcher             = \"%s\"\n  }\n", a.Healthy, a.Unhealthy, a.Matcher)
	if a.Sticky {
		w("\n  stickiness {\n    type            = \"lb_cookie\"\n    cookie_duration = %d\n    enabled         = true\n  }\n", a.StickySecs)
	}
	w("}\n\n")
	w("resource \"aws_lb_listener\" \"%s_http\" {\n  load_balancer_arn = aws_lb.%s.arn\n  port              = 80\n  protocol          = \"HTTP\"\n\n", res, res)
	w("  default_action {\n    type             = \"forward\"\n    target_group_arn = aws_lb_target_group.%s.arn\n  }\n}\n", res)
	return b.String(), a.Warnings
}
