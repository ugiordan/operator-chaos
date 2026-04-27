# CLI Reference

Auto-generated from cobra command definitions.

## operator-chaos

Chaos engineering framework for Kubernetes operators

### Synopsis

Operator Chaos tests operator reconciliation semantics.
It validates that operators recover managed resources correctly after
fault injection, not just that pods restart.

### Options

```
  -h, --help                help for operator-chaos
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos analyze

Analyze Go source code for fault injection candidates

```
operator-chaos analyze <directory> [flags]
```

### Options

```
  -h, --help   help for analyze
      --json   output in JSON format
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos clean

Remove all chaos artifacts from the cluster (emergency stop)

```
operator-chaos clean [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos controller

Controller mode commands

### Options

```
  -h, --help   help for controller
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos controller start

Start the ChaosExperiment controller

### Synopsis

Starts a Kubernetes controller that watches ChaosExperiment CRs and drives them through the experiment lifecycle.

```
operator-chaos controller start [flags]
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

## operator-chaos diff-crds

Compare CRD schemas between versions

### Synopsis

Compare OpenAPI v3 schemas embedded in CRD YAML files. Detects field removals, type changes, enum value changes, defaulting shifts, and API version removals.

```
operator-chaos diff-crds [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos diff

Compare two versioned knowledge model directories

### Synopsis

Structural comparison of operator knowledge models between two versions. Detects renames, namespace moves, webhook changes, and dependency shifts. No cluster access required.

```
operator-chaos diff [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos generate

Generate code from knowledge models

### Options

```
  -h, --help   help for generate
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos generate chaos

Generate a chaos playbook from knowledge models and experiments

```
operator-chaos generate chaos [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos generate fuzz-targets

Generate fuzz test targets from a knowledge model

```
operator-chaos generate fuzz-targets [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos generate upgrade

Generate an upgrade playbook from knowledge directories

```
operator-chaos generate upgrade [flags]
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

## operator-chaos init

Generate a skeleton experiment YAML

```
operator-chaos init [flags]
```

### Options

```
      --component string   target component name (required)
  -h, --help               help for init
      --namespace string   target namespace (default "default")
      --operator string    target operator (required)
      --type string        injection type (PodKill|NetworkPartition|CRDMutation|ConfigDrift|WebhookDisrupt|RBACRevoke|FinalizerBlock|ClientFault|OwnerRefOrphan|QuotaExhaustion|WebhookLatency|NamespaceDeletion|LabelStomping) (default "PodKill")
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
  -v, --verbose             verbose output
```

#

---

## operator-chaos playbook

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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos playbook run

Execute a playbook (UpgradePlaybook or ChaosPlaybook)

```
operator-chaos playbook run [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos preflight

Check cluster readiness before running chaos experiments

### Synopsis

Preflight verifies that all resources declared in an operator knowledge
file exist and are healthy on the cluster. Use --local to validate the
knowledge file structure without connecting to a cluster.

```
operator-chaos preflight [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos report

Generate reports from experiment results

### Synopsis

Reads JSON experiment results from a directory and generates reports in the specified format.

```
operator-chaos report <results-directory> [flags]
```

### Options

```
      --format string   output format (summary, json, junit, html, markdown) (default "summary")
  -h, --help            help for report
      --output string   output file path (default: stdout for summary/markdown, auto-named file for json/junit/html)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos run

Run a chaos experiment

```
operator-chaos run <experiment.yaml> [flags]
```

### Options

```
      --distributed-lock        use Kubernetes Lease-based distributed locking
      --dry-run                 validate without injecting
  -h, --help                    help for run
      --knowledge stringArray   path to operator knowledge YAML (repeatable)
      --knowledge-dir string    directory of operator knowledge YAMLs
      --lock-namespace string   namespace for distributed lock leases (default "default")
      --max-tier int32          skip experiments above this tier (0 = no filter)
      --report-dir string       directory for report output
      --timeout duration        total experiment timeout (default 10m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos simulate-upgrade

Simulate an upgrade by computing diff and generating experiments

### Synopsis

Compares source and target versioned knowledge directories, computes
the structural diff, and generates chaos experiments that simulate the
effects of each detected change. Use --dry-run to preview the generated
experiments without executing them.

```
operator-chaos simulate-upgrade [flags]
```

### Options

```
      --component string        limit to a specific component
      --distributed-lock        use Kubernetes Lease-based distributed locking
      --dry-run                 output generated experiments without executing
  -h, --help                    help for simulate-upgrade
      --knowledge stringArray   path to operator knowledge YAML (for live execution)
      --knowledge-dir string    directory of operator knowledge YAMLs (for live execution)
      --lock-namespace string   namespace for distributed lock leases (default "default")
      --report-dir string       directory for reports
      --source string           path to source version knowledge directory (required)
      --target string           path to target version knowledge directory (required)
      --timeout duration        timeout per experiment (default 10m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos suite

Run all experiments in a directory

```
operator-chaos suite <experiments-directory> [flags]
```

### Options

```
      --distributed-lock        use Kubernetes Lease-based distributed locking
      --dry-run                 validate without running
  -h, --help                    help for suite
      --knowledge stringArray   path to operator knowledge YAML (repeatable)
      --knowledge-dir string    directory of operator knowledge YAMLs
      --lock-namespace string   namespace for distributed lock leases (default "default")
      --max-tier int32          skip experiments above this tier (0 = no filter)
      --parallel int            max concurrent experiments (default 1)
      --report-dir string       directory for report output
      --timeout duration        timeout per experiment (default 10m0s)
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos types

List available injection types

```
operator-chaos types [flags]
```

### Options

```
  -h, --help   help for types
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos upgrade

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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos upgrade discover

Show available OLM channels and versions for an operator

```
operator-chaos upgrade discover [flags]
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

## operator-chaos upgrade monitor

Watch an in-progress OLM upgrade

```
operator-chaos upgrade monitor [flags]
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

## operator-chaos upgrade run

Execute an upgrade playbook

```
operator-chaos upgrade run [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos upgrade trigger

Trigger a single OLM channel hop

```
operator-chaos upgrade trigger [flags]
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

## operator-chaos validate-version

Validate versioned knowledge models against a live cluster

### Synopsis

Loads all knowledge models from a versioned directory and checks that
the expected managed resources exist on the cluster. Useful for verifying
that a knowledge directory accurately describes the current cluster state.

```
operator-chaos validate-version [flags]
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
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos validate

Validate experiment or knowledge YAML without running

```
operator-chaos validate <file.yaml> [flags]
```

### Options

```
  -h, --help        help for validate
      --knowledge   validate an OperatorKnowledge YAML file instead of an experiment
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

## operator-chaos version

Print the version

```
operator-chaos version [flags]
```

### Options

```
  -h, --help   help for version
```

### Options inherited from parent commands

```
      --kubeconfig string   path to kubeconfig file
      --namespace string    target namespace (default "default")
  -v, --verbose             verbose output
```

#

---

