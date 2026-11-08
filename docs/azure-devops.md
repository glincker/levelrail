---
title: "Azure DevOps"
description: "Deploy from Azure Repos with a personal access token and a push service hook."
---

Azure DevOps Services (`dev.azure.com`) and Azure DevOps Server connect with a personal access token. There is no OAuth application to register and no public domain needed to connect.

## Connect

Create a token under User settings, Personal access tokens, with the **Code (Read)** scope. Add **Code (Read & write)** only if you want the control plane to register the push hook for you (service hooks need project administrator rights on the project).

```bash
levelrail-cli git-providers test --type azure_devops --instance-url https://dev.azure.com/ORG --pat TOKEN
levelrail-cli git-providers add --type azure_devops --instance-url https://dev.azure.com/ORG --pat TOKEN
```

The instance URL is the organization root. One connection covers every project in that organization. Connect each organization separately. The token is stored encrypted and never returned by the API.

## Use a repository as a source

Repositories are addressed as `PROJECT/repo`:

```bash
levelrail-cli git-providers repos azure_devops
levelrail-cli git-providers branches --id CONNECTION_ID --repo Shop/app
levelrail-cli git-providers use-as-source --id CONNECTION_ID --repo Shop/app --app-name shop
```

The same calls are available over the REST API under `/api/v1/git-connections` with `"provider": "azure_devops"`.

## Push webhook

The control plane registers a `git.push` service hook that posts to the app's webhook URL. Service hooks cannot sign payloads, so the shared secret travels as the basic-auth password and is compared in constant time. If you create the hook by hand, use the basic-auth user `webhook` and the app's webhook secret as the password, event **Code pushed**.

Only branch pushes deploy. Tag pushes and branch deletions are acknowledged and ignored.

## Limits

- Pull request previews and commit status reporting are not supported on Azure DevOps yet.
- Beta: verified against recorded payloads, not yet against a live organization.
