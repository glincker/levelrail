# levelrail-cli

Command line client for [Levelrail](https://levelrail.com), a self-hosted deployment platform.

```
npx levelrail-cli help
npm install --global levelrail-cli
```

Point it at your instance and log in:

```
export APP_API_URL=https://your-dashboard-domain
levelrail-cli auth login --device
```

This package installs a prebuilt binary for your platform (Linux, macOS, or Windows on x64 or arm64) through optional dependencies. Releases are checksummed and signed; see [Installing](https://levelrail.com/installing) for how to verify them. Full command list: [CLI reference](https://levelrail.com/cli-reference).

Apache 2.0.
