---
marp: true
theme: default
paginate: true
backgroundColor: #fff
style: |
  section { font-size: 28px; }
  h1 { color: #c00; font-size: 42px; }
  h2 { color: #333; font-size: 36px; }
  code { font-size: 22px; }
  table { font-size: 22px; }
  .columns { display: flex; gap: 2em; }
  .col { flex: 1; }
---

# ODH Platform Chaos

## Semantic Chaos Engineering for OpenDataHub Operators

**Architects Council Presentation**
March 2026

---

## The Problem We Solve

Traditional chaos tools (LitmusChaos, Chaos Mesh) answer:
> "Does the pod come back after being killed?"

**We answer a harder question:**
> "After a fault, does the operator **semantically restore** all managed resources — with correct specs, owner references, labels, and reconciliation state?"

A pod restarting is necessary but **not sufficient**. An operator can restart and still leave ConfigMaps drifted, webhooks misconfigured, or RBAC bindings empty.

---

## Why This Matters for RHOAI

RHOAI operators manage **complex resource graphs**:

| Operator | Managed Resources | Webhooks | Finalizers |
|----------|:-:|:-:|:-:|
| odh-model-controller | 6 | 7 | 3 |
| kserve | 16 | 12 | 4 |

*(Counts derived from operator knowledge models — verified against deployed manifests.)*

A single unreconciled ConfigMap (`inferenceservice-config`) can silently break **all InferenceService deployments** across the cluster without any pod restart. We'll demonstrate this specific scenario later with a real experiment result.

**This tool catches those failures before users do.**

---

## Who Uses This and When

| Persona | Usage Mode | When |
|---------|-----------|------|
| **Operator developer** | SDK middleware in integration tests | Every PR — catches regression in error handling |
| **QE engineer** | CLI `suite` command against staging | Release gating — proves reconciliation under fault |
| **SRE / on-call** | CLI `run` against production-like env | Incident prep — validates recovery playbooks |
| **CI pipeline** | Container image + JUnit reports | Automated — blocks promotion on failure |

**Common thread:** each persona needs to verify that the operator **semantically heals**, not just that pods restart.

---

## Why Build, Not Extend?

| Requirement | LitmusChaos | Chaos Mesh | ODH Platform Chaos |
|-------------|:-:|:-:|:-:|
| Semantic reconciliation verification | No | No | **Yes** |
| Knowledge model (operator-specific oracle) | No | No | **Yes** |
| SDK middleware for integration tests | No | No | **Yes** |
| Fuzz testing (no cluster needed) | No | No | **Yes** |
| Lightweight (no CRD operator to install) | No | No | **Yes** |
| Pod/network fault injection | Yes | Yes | Yes |

**LitmusChaos and Chaos Mesh are infrastructure chaos tools.** They verify platform resilience (node failures, network partitions). They do not understand what an operator *should* reconcile after a fault.

**We are an operator correctness tool.** We encode what "healed correctly" means in a knowledge model and verify it. The two categories are complementary, not competing.

---

## Architecture Overview

![Architecture Overview](images/Architecture%20Overview.png)

All components injected via `OrchestratorConfig` — no globals, no singletons, fully testable. The framework itself has unit and integration tests across all packages with >80% line coverage.

---

## Experiment Lifecycle — State Machine

![Experiment Lifecycle](images/Experiment%20Lifecycle.png)

| Phase | What Happens | Interruptible? |
|-------|-------------|:--:|
| **Pending** | Validate blast radius, acquire lock, check danger level | Yes — no mutation yet |
| **SteadyStatePre** | Baseline health check (conditions, resource existence) | Yes — no mutation yet |
| **Injecting** | Execute fault via Injector interface | Yes — cleanup runs |
| **Observing** | Wait for reconciliation (knowledge-driven) | Interrupt triggers cleanup |
| **SteadyStatePost** | Re-verify health after recovery window | Interrupt triggers cleanup |
| **Evaluating** | Compare pre/post, determine verdict | Interrupt triggers cleanup |
| **Complete** | Write report, release lock, cleanup | Cleanup already running |

**Key invariant:** Cleanup runs in `defer` — even on context cancellation, timeout, or panic. An interrupted experiment always rolls back; it just skips the evaluation phase. (Note: `defer` does not survive SIGKILL or OOM-kill — rollback annotations on the resources allow the `clean` command to recover in those cases.)

---

## Injection Registry — Strategy Pattern

```go
type Injector interface {
    Validate(spec InjectionSpec, blast BlastRadiusSpec) error
    Inject(ctx context.Context, spec InjectionSpec, namespace string)
           (CleanupFunc, []InjectionEvent, error)
}

type CleanupFunc func(ctx context.Context) error
```

Every injector returns a `CleanupFunc` that **reverses the fault**. The orchestrator calls cleanup in `defer`, guaranteeing rollback even on failure.

**Why an interface?** Each injection type has fundamentally different K8s API interactions (pods vs NetworkPolicies vs RBAC vs webhooks). A common interface enables the registry pattern while allowing type-specific validation.

---

## The Seven Injection Types

| Type | What It Does | Danger |
|------|-------------|:------:|
| **PodKill** | Force-delete pods (0s grace) | Low |
| **NetworkPartition** | Deny-all NetworkPolicy | Medium |
| **ConfigDrift** | Modify ConfigMap/Secret data | Medium |
| **CRDMutation** | Patch custom resource fields | Medium |
| **WebhookDisrupt** | Change webhook failure policy | High |
| **RBACRevoke** | Clear binding subjects | High |
| **FinalizerBlock** | Add blocking finalizer | Medium |

**Five operator failure categories covered:**
1. **Pod lifecycle** — 2. **Network** — 3. **Configuration** — 4. **Control plane** — 5. **Lifecycle hooks**

---

## Safety Architecture — Defense in Depth

![Safety Architecture](images/Safety%20Architecture.png)

**Design principle:** Every mutation must be reversible. Every artifact must be traceable. No silent state corruption.

**Parallel experiments:** The distributed lock is per-operator (one Lease per operator name). Experiments targeting different operators can run concurrently. Same-operator experiments are serialized to prevent conflicting mutations.

---

## Security, Authorization & Audit

**Who can run experiments?**
The chaos tool runs as a regular Kubernetes client. Authorization is enforced by **standard RBAC** — whoever runs the tool must have permissions on the target resources.

**Required ClusterRole permissions:**

| Injection Type | Permissions Needed |
|---------------|-------------------|
| PodKill | `delete` on pods |
| ConfigDrift | `get`, `patch` on configmaps/secrets |
| WebhookDisrupt | `get`, `patch` on validating/mutatingwebhookconfigurations |
| RBACRevoke | `get`, `patch` on clusterrolebindings |
| DistributedLock | `create`, `get`, `update` on leases |
| Reports | `create`, `update` on configmaps |

**Why no custom auth layer?** Kubernetes RBAC already solves authorization. Adding a second layer creates false security — RBAC is the security boundary.

**Audit trail:** Every experiment produces a report ConfigMap with the experiment name, timestamp, verdict, injection details, and recovery metrics. These ConfigMaps are labeled and queryable. Kubernetes audit logs capture which ServiceAccount created them, providing full provenance. The `clean` command removes only artifacts with the `managed-by=odh-chaos` label.

---

## Knowledge Model — Encoding Operator Semantics

```yaml
operator:
  name: odh-model-controller
  namespace: opendatahub
components:
  - name: odh-model-controller
    controller: DataScienceCluster
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        expectedSpec:
          replicas: 1
    webhooks:
      - name: validating.isvc.odh-model-controller
        type: validating
    finalizers:
      - odh.inferenceservice.finalizers
    steadyState:
      checks:
        - type: conditionTrue
          kind: Deployment
          name: odh-model-controller
          conditionType: Available
recovery:
  reconcileTimeout: "300s"
  maxReconcileCycles: 10
```

---

## What the Knowledge Model Enables

Without it, chaos testing is **blind**: kill a pod and check if it restarts.

With it, we verify:
- Did the operator reconcile **all 6 managed resources** (not just the Deployment)?
- Are owner references intact?
- Is `expectedSpec.replicas` still correct after recovery?
- Did the webhook get re-registered?
- Was the recovery within the **300s timeout** and **10 cycle limit**?

**The knowledge model turns chaos testing from "did it crash?" to "did it heal correctly?"**

**Maintenance:** A typical knowledge model is 30-80 lines of YAML per operator. It is versioned alongside operator code and validated by `preflight --local` before experiments run. Updates are needed when an operator adds or removes managed resources — roughly once per release cycle.

**OLM interaction:** For OLM-managed operators, the knowledge model references resources the operator creates (not those OLM creates from the CSV), avoiding conflicts with OLM's own reconciliation loop.

**Current expressiveness limits:** The model handles resource existence, field values, conditions, and owner references. It does not yet express cross-resource invariants (e.g., "Service selector must match Deployment labels") or temporal ordering. These can be added as `customCheck` types if needed.

---

## Verdict Engine — Decision Tree

![Verdict Engine](images/Verdict%20Engine.png)

**Four verdicts, not two.** Binary pass/fail loses information. `Degraded` tells you "it recovered, but not well enough" — actionable for optimization without blocking releases.

---

## Three Usage Modes

![Three Usage Modes](images/Three%20Usage%20Modes.png)

Let's zoom into the SDK mode — how operators can test resilience without a cluster.

---

## SDK Middleware — Testing Without a Cluster

```go
// Wrap any controller-runtime client with fault injection
chaosClient := sdk.NewChaosClient(realClient, faultConfig)

// Every K8s API call goes through MaybeInject()
func (c *ChaosClient) Get(ctx, key, obj, opts...) error {
    if err := c.faults.MaybeInject(OpGet); err != nil {
        return err  // Injected fault
    }
    return c.inner.Get(ctx, key, obj, opts...)
}
```

**ChaosClient implements `client.Client`** — drop-in replacement. No code changes needed in the operator under test.

**WrapReconciler** does the same for the reconciler itself — intercept before delegation, inject faults probabilistically into real code paths.

**Performance:** `MaybeInject()` is a single map lookup + random check per call — negligible overhead. No reflection, no deep copy.

**Why middleware, not mocks?** Mocks require knowing the exact call sequence. Middleware injects faults probabilistically into real code paths, testing actual error handling.

---

## Concrete Example: ConfigDrift on inferenceservice-config

```yaml
metadata:
  name: config-drift-isvc-config
spec:
  injection:
    type: ConfigDrift
    target:
      name: inferenceservice-config
      namespace: opendatahub
    parameters:
      key: "config"
      value: '{"corrupted": true}'
  blastRadius:
    allowedNamespaces: [opendatahub]
    maxPodsAffected: 0
```

**What this tests:** After corrupting the `inferenceservice-config` ConfigMap, does odh-model-controller detect the drift and restore it?

**What we found:** In early testing, the controller **did not reconcile this ConfigMap** — it is created during installation but not watched for drift. This is a real gap: any manual edit or accidental overwrite silently breaks all InferenceService deployments. This finding drove a reconciler fix upstream.

---

## Tool Failure Modes — What If We Fail?

| Failure | Mitigation |
|---------|-----------|
| Tool crashes mid-injection | Cleanup in `defer` — runs on panic. For SIGKILL/OOM (where `defer` doesn't execute), rollback data in annotations survives — the `clean` command restores them. |
| Rollback annotation corrupted | SHA-256 checksum — refuses to apply, logs for manual recovery. |
| Lock lease not released | 15-minute auto-expiry via `leaseDurationSeconds`. |
| Tool loses cluster connectivity | `defer` cleanup attempts rollback. If unreachable, rollback annotations persist on the resource — the `clean` command restores them on next invocation. TTL annotation marks artifacts for garbage collection. |
| Knowledge model is wrong | `preflight` validates knowledge against live cluster before any experiment. |

**Design principle:** The tool must never make things worse than the fault it injected. Every failure mode has at least one recovery path that does not require human intervention.

---

## CI Integration & Reports

**Exit code contract for CI pipelines:**

| Command | Exit 0 | Non-zero |
|---------|--------|----------|
| `preflight` | All resources found & healthy | Missing or unreachable |
| `run` | Verdict is Resilient | Degraded, Failed, or Inconclusive |
| `suite` | All experiments pass | Any experiment failed |
| `validate` | YAML is valid | Validation errors |

**Report destinations:** JSON files, JUnit XML (CI-native), K8s ConfigMaps (queryable via `kubectl`)

```bash
# Find all failed experiments on the cluster
kubectl get cm -l chaos.opendatahub.io/verdict=failed
```

Container: distroless/static:nonroot — no shell, non-root UID 65532, multi-arch (amd64/arm64). Tekton Tasks and GitHub Actions workflows provided in `docs/ci-integration-guide.md`.

---

## What We Don't Do (Deliberate Non-Goals)

| Non-Goal | Reasoning |
|----------|-----------|
| CRD-based controller | CLI-first avoids operator lifecycle complexity. CRD mode planned once experiment API stabilizes — adding it now would force premature API commitment. |
| Real-time dashboard | Reports are queryable via ConfigMaps + JUnit. A UI requires a running service — contradicts our "run and exit" model. |
| Network-level injection (iptables/tc) | NetworkPolicy is the K8s-native abstraction. iptables requires privileged containers, conflicting with our non-root security model. |
| Multi-cluster support | Single-cluster keeps the safety model simple. Cross-cluster would require distributed locking across clusters — a fundamentally different coordination problem. |
| Mutation webhooks for injection | Too invasive. Webhook-based injection intercepts all writes — blast radius cannot be bounded. Direct API mutations are fully reversible. |

---

## Roadmap & Ask

| Phase | Status | What |
|-------|--------|------|
| Core framework | Done | 7 injectors, orchestrator, evaluator, safety |
| SDK middleware | Done | ChaosClient, WrapReconciler, TestChaos, FuzzTesting |
| CLI & CI | Done | 10 commands, JUnit, container image, CI guide |
| **Next: Operator coverage** | Planned | Knowledge models for all RHOAI operators |
| **Next: CRD mode** | Planned | Kubernetes-native ChaosExperiment CRD |
| **Next: OpenShift CI** | Planned | Integration into RHOAI release gating pipeline |

**Our ask to this council:**

1. **Adopt as a recommended practice** — pilot with 2-3 operators this quarter, then evaluate for standardization based on results
2. **Run advisory-mode chaos suites** — integrate into CI as informational for 2 release cycles before gating on results
3. **Feedback on operator coverage** — which operators should we model first beyond odh-model-controller and kserve?

---

## Summary

**ODH Platform Chaos** tests what matters: **semantic reconciliation correctness**, not just pod restarts.

**Key architectural decisions:**
1. **Knowledge-driven** — operator semantics encoded in YAML, not hardcoded
2. **Safety-first** — 6 layers: blast radius, danger gates, distributed lock, TTL, checksummed rollback, defer cleanup
3. **Interface-driven** — Injector, Observer, Lock are all pluggable
4. **Three modes** — CLI (cluster), SDK (integration), Fuzz (unit)
5. **CI-native** — exit codes, JUnit, container image, Tekton/GHA examples
6. **RBAC-delegated security** — no custom auth, Kubernetes is the security boundary

**One question to take away:**
> For each RHOAI operator, can we prove that after any of these 7 fault types, the operator restores **all** managed resources to their correct state within the expected time?

---
