---
title: "Use AWS EC2 with Levelrail: node provisioning"
sidebarTitle: "AWS EC2"
description: "Let Levelrail create a server at AWS EC2 with a first-boot script that installs Docker and enrolls the node agent."
---

Let Levelrail create a server at AWS EC2 with a first-boot script that installs Docker and enrolls the node agent.

This page covers how to create servers and enroll them as nodes.

## What you get

- Create a server at AWS EC2 from the Add node wizard or the CLI, with live region and size pickers.
- A cloud-init script that installs Docker if missing and starts the agent, which dials out to the control plane.
- Provision status from creating through booting and enrolling to ready, or failed with a reason.

## Set up

1. Prepare the credential. Choose a method: access_key (an IAM access key id and secret), assume_role (a role ARN, optionally with a base key) or ambient (the control plane's own AWS identity).
2. In the dashboard, open **Settings > Cloud node providers** (`/settings/node-providers?connect=aws`), then Nodes > Add node.
3. Connect runs a read-only region listing first and stores nothing when it fails, so a saved connection already works.

From the CLI:

```bash
levelrail-cli nodes providers connect --provider aws --method access_key --provider-token <ACCESS_KEY_ID> --secret-access-key <SECRET> --region us-east-1
levelrail-cli nodes provision --provider aws --region <REGION> --size <SIZE> --name <NODE_NAME>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| ec2:RunInstances, TerminateInstances | Create and delete the instance. |
| ec2:CreateSecurityGroup, AuthorizeSecurityGroupIngress, RevokeSecurityGroupIngress, DeleteSecurityGroup | Manage the shared security group, which keeps zero inbound rules. |
| ec2:CreateTags, ec2:Describe* | Tag instances and look up regions, instance types, images, VPCs and subnets. |
| sts:AssumeRole | Only for the assume_role method: the base identity needs this on the role, and the role carries the EC2 actions. |

## Endpoint and format

Default region `us-east-1` unless you pass --region. Optional --session-token and --role-arn.

## Security notes

- A saved credential is envelope encrypted and never shown again, in the API or the audit log.
- Piping a credential on stdin avoids leaving it in shell history; a flag value is visible in the process list.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The target region needs a default VPC. There is no option yet to name a VPC or subnet.
- The size list is a curated set of t3 types, t3.micro through t3.xlarge.
- Full OIDC federation with AssumeRoleWithWebIdentity is not supported.
- --allow-ssh-inbound opens TCP 22 to any source on a dedicated security group, and that group is removed only when a failed provision rolls back.

## Troubleshooting

**Connect refuses to store the credential.** Connect runs a read-only region listing first and stores nothing when it fails. Check the key can call ec2:Describe*. With a role ARN, the base key needs sts:AssumeRole on that role.

**A provision sits in enrolling.** The server booted but the agent has not dialed home. Read /var/log/cloud-init-output.log from the provider console; the provision is marked failed after APP_NODE_PROVISION_TIMEOUT (default 20 minutes).

**A name conflict returns 409.** The node name is already used by an enrolled node or an unfinished provision. Pick another name.

## FAQ

**Can the control plane use its own AWS identity?**

Yes, with --use-ambient-credentials, which resolves the environment, shared config or an instance profile. It only makes sense when the control plane runs on AWS.

**Does it open SSH?**

Not by default. The shared security group has no inbound rules, matching the no inbound ports design.

## Related

- [Amazon S3](/integrations/backup-destinations/aws): Backup destinations
- [AWS ECR](/integrations/container-registries/ecr): Container registries
- [All cloud node providers](/integrations/cloud-providers)
- [AWS EC2 documentation](https://docs.aws.amazon.com/ec2/), maintained by the vendor
