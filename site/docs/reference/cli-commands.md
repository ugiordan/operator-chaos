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

