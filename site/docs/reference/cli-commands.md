# CLI Reference

Auto-generated from cobra command definitions.

## odh-chaos

Chaos engineering framework for OpenDataHub operators

### Synopsis

ODH Platform Chaos tests operator reconciliation semantics.
It validates that operators recover managed resources correctly after
fault injection, not just that pods restart.

### Options

```
  -h, --help                help for odh-chaos
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos analyze

Analyze Go source code for fault injection candidates

```
odh-chaos analyze <directory> [flags]
```

### Options

```
  -h, --help   help for analyze
      --json   output in JSON format
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos clean

Remove all chaos artifacts from the cluster (emergency stop)

```
odh-chaos clean [flags]
```

### Options

```
  -h, --help                help for clean
      --interval duration   scan interval when --watch is set (default 1m0s)
      --watch               continuously scan and clean chaos artifacts
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos controller

Controller mode commands

### Options

```
  -h, --help   help for controller
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos controller start

Start the ChaosExperiment controller

### Synopsis

Starts a Kubernetes controller that watches ChaosExperiment CRs and drives them through the experiment lifecycle.

```
odh-chaos controller start [flags]
```

### Options

```
      --health-addr string     health probe bind address (default ":8081")
  -h, --help                   help for start
      --knowledge-dir string   directory of operator knowledge YAMLs
      --leader-elect           enable leader election (default true)
      --metrics-addr string    metrics bind address (default ":8080")
      --namespace string       namespace to watch (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## odh-chaos diff-crds

Compare CRD schemas between versions

### Synopsis

Compare OpenAPI v3 schemas embedded in CRD YAML files. Detects field removals, type changes, enum value changes, defaulting shifts, and API version removals.

```
odh-chaos diff-crds [flags]
```

### Options

```
      --format string        output format: table, json, yaml (default "table")
  -h, --help                 help for diff-crds
      --source-crds string   path to source version CRD YAML directory
      --target-crds string   path to target version CRD YAML directory (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos diff

Compare two versioned knowledge model directories

### Synopsis

Structural comparison of operator knowledge models between two versions. Detects renames, namespace moves, webhook changes, and dependency shifts. No cluster access required.

```
odh-chaos diff [flags]
```

### Options

```
      --breaking        only show breaking changes
      --format string   output format: table, json, yaml (default "table")
  -h, --help            help for diff
      --source string   path to source version knowledge directory (required)
      --target string   path to target version knowledge directory (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos generate

Generate code from knowledge models

### Options

```
  -h, --help   help for generate
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos generate chaos

Generate a chaos playbook from knowledge models and experiments

```
odh-chaos generate chaos [flags]
```

### Options

```
      --danger string        danger filter: all, low, medium, high (default "all")
      --experiments string   experiments directory (required)
  -h, --help                 help for chaos
      --knowledge string     knowledge directory (required)
      --output string        output file path (defaults to stdout)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos generate fuzz-targets

Generate fuzz test targets from a knowledge model

```
odh-chaos generate fuzz-targets [flags]
```

### Options

```
  -h, --help               help for fuzz-targets
      --knowledge string   path to knowledge YAML file (required)
      --output string      output file path (defaults to stdout)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos generate upgrade

Generate an upgrade playbook from knowledge directories

```
odh-chaos generate upgrade [flags]
```

### Options

```
      --discover           try to discover OLM channels from cluster (default true)
  -h, --help               help for upgrade
      --namespace string   operator namespace (defaults to first model's namespace)
      --operator string    operator name (defaults to first model's operator)
      --output string      output file path (defaults to stdout)
      --source string      source knowledge directory (required)
      --target string      target knowledge directory (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## odh-chaos init

Generate a skeleton experiment YAML

```
odh-chaos init [flags]
```

### Options

```
      --component string   target component name (required)
  -h, --help               help for init
      --namespace string   target namespace (default "opendatahub")
      --operator string    target operator (default "opendatahub-operator")
      --type string        injection type (PodKill|NetworkPartition|CRDMutation|ConfigDrift|WebhookDisrupt|RBACRevoke|FinalizerBlock|ClientFault) (default "PodKill")
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## odh-chaos playbook

Execute upgrade and chaos playbooks

### Synopsis

Run multi-step playbooks that orchestrate upgrades, chaos experiments,
and validation steps. Supports both UpgradePlaybook and ChaosPlaybook kinds.

### Options

```
  -h, --help   help for playbook
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos playbook run

Execute a playbook (UpgradePlaybook or ChaosPlaybook)

```
odh-chaos playbook run [flags]
```

### Options

```
      --allow-shell          Allow kubectl steps with shell commands
      --dry-run              Print execution plan without running
      --force-resume         Allow resume without state file
  -h, --help                 help for run
      --playbook string      Path to playbook YAML (required)
      --report-dir string    Directory for reports
      --resume-from string   Resume from failed step
      --skip-manual          Use autoCheck for manual steps in CI
      --state-dir string     Directory for state files
      --timeout duration     Overall timeout (default 1h0m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos preflight

Check cluster readiness before running chaos experiments

### Synopsis

Preflight verifies that all resources declared in an operator knowledge
file exist and are healthy on the cluster. Use --local to validate the
knowledge file structure without connecting to a cluster.

```
odh-chaos preflight [flags]
```

### Options

```
  -h, --help               help for preflight
      --knowledge string   path to operator knowledge YAML (required)
      --local              skip cluster checks, only validate knowledge file
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos report

Generate summary reports from experiment results

```
odh-chaos report <results-directory> [flags]
```

### Options

```
      --format string   output format (summary, junit) (default "summary")
  -h, --help            help for report
      --output string   output directory for reports
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos run

Run a chaos experiment

```
odh-chaos run <experiment.yaml> [flags]
```

### Options

```
      --distributed-lock        use Kubernetes Lease-based distributed locking
      --dry-run                 validate without injecting
  -h, --help                    help for run
      --knowledge stringArray   path to operator knowledge YAML (repeatable)
      --knowledge-dir string    directory of operator knowledge YAMLs
      --lock-namespace string   namespace for distributed lock leases (default "opendatahub")
      --report-dir string       directory for report output
      --timeout duration        total experiment timeout (default 10m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

!!! note "Full namespace override"
    The `--namespace` flag performs a comprehensive override when used with `run` or `suite` commands. It updates: (1) the experiment's metadata namespace, (2) all steady-state check namespaces, (3) the blast radius `allowedNamespaces` list, and (4) the reconciliation checker namespace. This allows the same experiment YAML files to work on both ODH (`opendatahub`) and RHOAI (`redhat-ods-applications`) clusters without modification.

    ```bash
    # Run an experiment written for ODH on a RHOAI cluster
    odh-chaos run experiments/odh-model-controller/pod-kill.yaml \
      --knowledge knowledge/odh-model-controller.yaml \
      --namespace redhat-ods-applications
    ```

#

---

## odh-chaos simulate-upgrade

Simulate an upgrade by computing diff and generating experiments

### Synopsis

Compares source and target versioned knowledge directories, computes
the structural diff, and generates chaos experiments that simulate the
effects of each detected change. Use --dry-run to preview the generated
experiments without executing them.

```
odh-chaos simulate-upgrade [flags]
```

### Options

```
      --component string    limit to a specific component
      --dry-run             output generated experiments without executing
  -h, --help                help for simulate-upgrade
      --report-dir string   directory for reports
      --source string       path to source version knowledge directory (required)
      --target string       path to target version knowledge directory (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos suite

Run all experiments in a directory

```
odh-chaos suite <experiments-directory> [flags]
```

### Options

```
      --distributed-lock        use Kubernetes Lease-based distributed locking
      --dry-run                 validate without running
  -h, --help                    help for suite
      --knowledge stringArray   path to operator knowledge YAML (repeatable)
      --knowledge-dir string    directory of operator knowledge YAMLs
      --lock-namespace string   namespace for distributed lock leases (default "opendatahub")
      --parallel int            max concurrent experiments (default 1)
      --report-dir string       directory for report output
      --timeout duration        timeout per experiment (default 10m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos types

List available injection types

```
odh-chaos types [flags]
```

### Options

```
  -h, --help   help for types
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos upgrade

OLM upgrade management and playbook execution

### Synopsis

Manage operator upgrades via OLM. Subcommands cover the full upgrade lifecycle:
discover available channels, trigger single-hop upgrades, monitor in-progress
upgrades, and execute multi-step upgrade playbooks.

### Options

```
  -h, --help   help for upgrade
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos upgrade discover

Show available OLM channels and versions for an operator

```
odh-chaos upgrade discover [flags]
```

### Options

```
      --format string      Output format: table, json (default "table")
  -h, --help               help for discover
      --namespace string   Subscription namespace (required)
      --operator string    OLM operator package name (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## odh-chaos upgrade monitor

Watch an in-progress OLM upgrade

```
odh-chaos upgrade monitor [flags]
```

### Options

```
  -h, --help               help for monitor
      --namespace string   Subscription namespace (required)
      --operator string    OLM operator package name (required)
      --timeout duration   Max watch time (default 30m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## odh-chaos upgrade run

Execute an upgrade playbook

```
odh-chaos upgrade run [flags]
```

### Options

```
      --allow-shell          Allow kubectl steps with shell commands
      --dry-run              Print execution plan without running
      --force-resume         Allow resume without state file
  -h, --help                 help for run
      --playbook string      Path to upgrade playbook YAML (required)
      --report-dir string    Directory for reports
      --resume-from string   Resume from failed step
      --skip-manual          Use autoCheck for manual steps in CI
      --state-dir string     Directory for state files
      --timeout duration     Overall timeout (default 1h0m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos upgrade trigger

Trigger a single OLM channel hop

```
odh-chaos upgrade trigger [flags]
```

### Options

```
      --channel string     Target OLM channel (required)
  -h, --help               help for trigger
      --namespace string   Subscription namespace (required)
      --operator string    OLM operator package name (required)
      --timeout duration   Max wait for CSV ready (default 20m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## odh-chaos validate-version

Validate versioned knowledge models against a live cluster

### Synopsis

Loads all knowledge models from a versioned directory and checks that
the expected managed resources exist on the cluster. Useful for verifying
that a knowledge directory accurately describes the current cluster state.

```
odh-chaos validate-version [flags]
```

### Options

```
      --format string          output format: table or json (default "table")
  -h, --help                   help for validate-version
      --knowledge-dir string   path to versioned knowledge directory (required)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos validate

Validate experiment or knowledge YAML without running

```
odh-chaos validate <file.yaml> [flags]
```

### Options

```
  -h, --help        help for validate
      --knowledge   validate an OperatorKnowledge YAML file instead of an experiment
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

## odh-chaos version

Print the version

```
odh-chaos version [flags]
```

### Options

```
  -h, --help   help for version
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "opendatahub")
  -v, --verbose             verbose output
```

#

---

