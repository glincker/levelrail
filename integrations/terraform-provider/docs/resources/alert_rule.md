---
page_title: "levelrail_alert_rule Resource - levelrail"
subcategory: "Apps"
description: |-
  An alert rule on one app, for example a metric threshold or a crash loop.
---

# levelrail_alert_rule (Resource)

An alert rule on one app, for example a metric threshold or a crash loop.

## Example Usage

```terraform
resource "levelrail_alert_rule" "web_crashloop" {
  app                     = levelrail_app.web.name
  name                    = "web crash loop"
  kind                    = "crashloop"
  restart_count_threshold = 3
}
```

## Argument Reference

### Required

- `app` (String) App name that owns the rule.
- `kind` (String) Rule kind, for example threshold or crashloop.
- `name` (String) Name of the resource.

### Optional

- `channel_id` (String) Notification channel to alert. Computed when unset.
- `comparator` (String) Comparator of a threshold rule. Computed when unset.
- `enabled` (Boolean) Whether the rule is active. Computed when unset.
- `for_duration` (String) How long the condition must hold, as a duration such as 5m. Computed when unset.
- `metric` (String) Metric a threshold rule watches. Computed when unset.
- `restart_count_threshold` (Number) Restarts that trip a crashloop rule. Computed when unset.
- `restart_window` (String) Window of a crashloop rule, as a duration. Computed when unset.
- `threshold` (Float64Attribute) Value that triggers the rule. Computed when unset.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by `app/rule_id`.

```shell
terraform import levelrail_alert_rule.example APP_NAME/RULE_ID
```
