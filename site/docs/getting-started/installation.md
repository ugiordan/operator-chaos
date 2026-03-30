# Installation

## Prerequisites

Before using ODH Platform Chaos, ensure you have the following:

### Required for All Modes

- **Go 1.25+** - Check `go.mod` in the repository for the exact version required
- **controller-runtime v0.23+** - Required for SDK and Fuzz modes

### Required for CLI and SDK Modes

- **Kubernetes/OpenShift cluster** - A live cluster is needed for CLI experiments and SDK middleware
  - Not required for fuzz testing, which uses a fake client
- **cluster-admin RBAC** - CLI experiments perform destructive operations including:
  - Pod deletion
  - RBAC revocation
  - Webhook mutation
  - NetworkPolicy creation

!!! warning "RBAC Requirements"
    CLI experiments require cluster-admin privileges because they perform intentionally destructive chaos operations. Never run experiments on production clusters without proper safeguards.

## Installation

Install the CLI using Go:

```bash
go install github.com/opendatahub-io/odh-platform-chaos/cmd/odh-chaos@latest
```

This will install the `odh-chaos` binary to your `$GOPATH/bin` directory.

## Container Images

Pre-built container images are available at:

```
quay.io/opendatahub/odh-chaos:latest
```

Use these images for running the chaos controller in Kubernetes or for CI/CD pipelines.

## Verify Installation

Check that the installation was successful:

```bash
odh-chaos version
```

You should see the version information for the installed CLI.

## Next Steps

Choose your usage mode based on your testing needs:

- **[CLI Quickstart](cli-quickstart.md)** - Run structured experiments against a live cluster
- **[SDK Quickstart](sdk-quickstart.md)** - Inject API-level faults in your operator code
- **[Fuzz Quickstart](fuzz-quickstart.md)** - Automated fault exploration during development
