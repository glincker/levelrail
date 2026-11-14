---
title: "Connect a cloud account"
tag: "Experimental"
description: Give Levelrail a scoped Oracle Cloud credential. A guided bootstrap from your own machine, or paste one you made. Root and tenancy admin credentials are never accepted.
---

<Warning>
Cloud connections are a preview. They stay off until the runtime setting `cloud.connections_enabled` is on. Cloud resources cost money: every cloud plan shows a cost estimate and needs an approval before it applies.
</Warning>

Levelrail never holds your admin login. It keeps one credential that can only manage compute, networking and buckets inside one compartment made for it.

## Pick an option

Safest first. Neither Oracle Cloud nor AWS offers a "sign in with" consent screen for managing resources, so there is no sign in button here.

| Option | What it touches | Limits |
| --- | --- | --- |
| 1. Guided bootstrap (`cloud connect oci`) | Your local `oci` CLI session, once, to create a compartment, a group, an API only user, a policy for that compartment and a key pair made on your machine | Needs the `oci` CLI and an admin session on your machine. The session never reaches Levelrail. |
| 2. Paste a scoped credential (`--paste`) | Only the user, compartment and key you made by hand | You create the user, group and policy yourself with the template below. |
| 3. No stored key | AWS OIDC role assumption, or an OCI instance principal when Levelrail runs on OCI | OCI federation with an outside issuer is not proven yet. |
| 4. Long lived keys | A key for a broader user | Discouraged. Only if 1 to 3 cannot work. |

Root or tenancy admin credentials are never accepted, and the tenancy root is never accepted as the compartment.

## Option 1: guided bootstrap

Sign in once with a browser session, then review the plan. The default is a dry run: it only reads your session and prints every command it would run.

```bash
oci session authenticate --profile-name DEFAULT --region us-phoenix-1
levelrail-cli cloud connect oci --name oci --region us-phoenix-1
```

Run it when the plan looks right:

```bash
levelrail-cli cloud connect oci --name oci --region us-phoenix-1 --yes
```

What it does, in order:

1. Creates the compartment `levelrail` under the tenancy root (or `--parent-compartment`).
2. Creates the group and the user `levelrail-<name>`, turns off the console password and every credential type except API keys, and adds the user to the group.
3. Creates the policy `levelrail-<name>` with statements for that one compartment. It never grants anything tenancy wide and never grants IAM rights.
4. Makes an RSA key pair on your machine, uploads only the public half and reads the fingerprint.
5. Sends tenancy, region, compartment, user, fingerprint and the private key to Levelrail over the authenticated API (https, or plain http only to this machine), then wipes the local copy.

If a step fails it stops, lists what was already created and prints the `oci` commands to remove it. Nothing is deleted for you. If your session expired it tells you to run `oci session authenticate` again.

Flags: `--profile` (default `DEFAULT`), `--region`, `--tenancy`, `--compartment-name` (default `levelrail`), `--parent-compartment`, `--name`, `--dry-run`, `--yes`.

## Option 2: paste a scoped credential

Create a group and an API only user, then apply this policy in the parent compartment. Replace the group name and compartment OCID.

```text
Allow group levelrail-oci to manage instance-family in compartment id ocid1.compartment.oc1..example
Allow group levelrail-oci to manage virtual-network-family in compartment id ocid1.compartment.oc1..example
Allow group levelrail-oci to manage buckets in compartment id ocid1.compartment.oc1..example
```

Then paste. The key comes from an environment variable or stdin, never from a flag. It is checked offline (OCID shapes, region, key size, fingerprint) before anything is sent.

```bash
export OCI_LEVELRAIL_KEY="$(cat ~/levelrail-oci.pem)"
levelrail-cli cloud connect oci --paste --name oci \
  --tenancy ocid1.tenancy.oc1..example --compartment-ocid ocid1.compartment.oc1..example \
  --region us-phoenix-1 --user-ocid ocid1.user.oc1..example \
  --fingerprint aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99 --key-env OCI_LEVELRAIL_KEY --yes
unset OCI_LEVELRAIL_KEY
```

## After connecting

```bash
levelrail-cli cloud connection test oci
levelrail-cli cloud connection list
```

Every cloud plan shows a cost estimate and waits for approval before it applies. See [How it works](/iac/how-it-works).
