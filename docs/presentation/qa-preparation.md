# Anticipated Q&A — ODH Platform Chaos Architects Council

## Category 1: Why This Tool?

### Q: "Why not just write better integration tests?"

**A:** Integration tests verify happy paths and known failure modes. Chaos testing discovers *unknown* failure modes by injecting faults the developer didn't anticipate. The ConfigDrift finding is the proof point — nobody wrote a test for "what if inferenceservice-config is externally modified?" because nobody expected it to happen. Chaos testing is about systematically testing assumptions you didn't know you were making.

### Q: "Can't we just use LitmusChaos/Chaos Mesh with custom probes?"

**A:** You could write custom probes, but you'd be reimplementing our knowledge model from scratch for every experiment. LitmusChaos probes check "is this HTTP endpoint healthy?" — they don't understand operator reconciliation semantics. You'd need to encode all managed resources, expected specs, owner references, and recovery timeouts in every probe script. That's what our knowledge model does declaratively in 30-80 lines of YAML per operator.

### Q: "This seems like a lot of infrastructure for a problem that might be rare."

**A:** The inferenceservice-config drift is not rare — it's undetected. Any manual kubectl edit, any GitOps drift, any script that touches operator-managed resources can trigger it. We just don't know how often it happens because we've never tested for it. The tool's value is in finding the gaps you don't know about.

---

## Category 2: Architecture & Design

### Q: "Why not use a CRD-based controller instead of a CLI?"

**A:** Two reasons. First, the experiment API isn't stable yet — committing to a CRD schema now would force premature API decisions. Second, a CLI-first approach is lighter: no operator lifecycle to manage, no CRD installation, no webhooks of our own. Once the API stabilizes, we'll add a CRD mode. The orchestrator is already decoupled from the CLI — it takes an experiment struct, not CLI flags.

### Q: "Why Kubernetes Leases for distributed locking instead of etcd directly or a separate coordination service?"

**A:** Leases are the Kubernetes-native coordination primitive — it's what leader election uses. They have built-in TTL semantics via `leaseDurationSeconds`, optimistic concurrency via `resourceVersion`, and no additional infrastructure requirements. Using etcd directly would bypass the API server's authorization model. A separate service (Consul, ZooKeeper) adds operational burden for a simple mutex.

### Q: "How does the Observer know when reconciliation is complete?"

**A:** It polls. The Observer checks steady-state conditions on an interval (configurable, default 5 seconds) until either all checks pass or the `reconcileTimeout` expires. It also counts reconcile cycles by watching resource `generation` and `observedGeneration` fields. This is intentionally simple — we avoid informers and watches because the tool is short-lived (minutes, not hours).

### Q: "Why SHA-256 checksums on rollback data? Isn't that overkill?"

**A:** Rollback data is stored in Kubernetes annotations on the mutated resources. Annotations can be manually edited — by operators, by scripts, by other controllers. Without checksum verification, a corrupted annotation would silently restore wrong state. The checksum catches this. SHA-256 is computationally negligible and prevents a class of silent corruption that would be extremely hard to debug.

### Q: "Why namespace whitelist instead of blacklist for blast radius?"

**A:** Blacklists are incomplete by definition. A new namespace created after the blacklist was written won't be blocked. Whitelists fail closed: if you didn't explicitly allow a namespace, it's denied. This is a security principle — deny by default, allow explicitly.

---

## Category 3: Knowledge Model

### Q: "Who owns the knowledge model? The operator team or the chaos team?"

**A:** The operator team, as part of their definition of done. They know what their operator reconciles — we don't. A knowledge model is 30-80 lines of YAML, less than a Dockerfile. If an operator team can't describe what their operator manages, that's a bigger problem than maintaining a YAML file.

### Q: "How do you validate that the knowledge model is correct?"

**A:** Three levels. First, `validate` checks YAML syntax and required fields. Second, `preflight --local` runs cross-reference checks — every steady-state check must reference a declared managed resource. Third, `preflight` (cluster mode) verifies every declared resource actually exists on the cluster and is healthy. If the knowledge model says there's a Deployment called `odh-model-controller` and it doesn't exist, preflight fails.

### Q: "What happens when the knowledge model drifts from the actual operator behavior?"

**A:** You get Inconclusive or Failed verdicts. If the knowledge model declares a resource that was removed from the operator, preflight catches it. If the operator added a new resource that isn't in the model, chaos testing won't verify it — but the model still correctly tests what it does declare. Knowledge model updates are needed roughly once per release cycle when operators change their managed resources.

### Q: "Can the knowledge model express cross-resource invariants?"

**A:** Not yet. Currently we handle resource existence, field values, conditions, and owner references. Cross-resource invariants (e.g., "Service selector must match Deployment labels") and temporal ordering constraints are not yet supported. These can be added as `customCheck` types — the check interface is extensible. We prioritized the checks that cover the most common failure modes first.

### Q: "How large is a typical knowledge model?"

**A:** odh-model-controller is 79 lines. kserve (with 4 sub-components) is about 200 lines. A typical single-component operator is 30-80 lines. The size scales linearly with the number of managed resources and steady-state checks.

---

## Category 4: Safety & Security

### Q: "What if the chaos tool has a bug and corrupts the cluster?"

**A:** Six layers of defense mitigate this. Most critically: every mutation stores the original state with a SHA-256 checksum, and cleanup runs in `defer` — even on panic. If the tool crashes, rollback annotations persist on the resources. The `clean` command or the next experiment invocation restores them. The tool is also tested with >80% line coverage across all packages.

### Q: "What is the blast radius of the tool itself compared to LitmusChaos?"

**A:** We're lighter. We run as a short-lived CLI process with only the RBAC permissions needed for the specific experiment. No CRDs, no operator pod, no webhooks, no persistent state beyond report ConfigMaps. LitmusChaos installs a full operator with its own CRDs, webhooks, and a runner infrastructure. Our attack surface is smaller because we have less running.

### Q: "How do you prevent someone from running dangerous experiments in production?"

**A:** Three mechanisms. First, RBAC — if the ServiceAccount doesn't have the required permissions, the experiment fails at the Kubernetes level. Second, blast radius whitelist — only explicitly allowed namespaces are targeted. Third, danger gates — high-danger injections (WebhookDisrupt, RBACRevoke) require `allowDangerous: true` in the experiment YAML. These are safety guardrails; RBAC is the security boundary.

### Q: "What audit trail is there for experiments?"

**A:** Every experiment writes a report ConfigMap with the experiment name, timestamp, verdict, injection type, recovery metrics, and any deviations. These ConfigMaps are labeled with `chaos.opendatahub.io/verdict={verdict}` and `managed-by=odh-chaos`. Kubernetes audit logs capture which ServiceAccount created them. You can query all failed experiments with `kubectl get cm -l chaos.opendatahub.io/verdict=failed`.

---

## Category 5: Operations & CI

### Q: "Can experiments run in parallel?"

**A:** Yes, with constraints. The distributed lock is per-operator — one Lease per operator name. Experiments targeting different operators run concurrently. Same-operator experiments are serialized to prevent conflicting mutations. The `suite` command supports `--parallel N` with a semaphore controlling concurrency.

### Q: "What happens if an OLM upgrade runs during an experiment?"

**A:** This is an edge case we acknowledge. OLM doesn't acquire our distributed lock, so an OLM upgrade during an active experiment could conflict. In practice, the blast radius is contained: our experiments target operator-reconciled resources, not CSV-managed resources. If OLM replaces the operator pod while we're observing recovery, the experiment will either see successful reconciliation (the new pod heals everything) or report Failed/Degraded (the timing caused incomplete recovery). The worst case is an inaccurate verdict, not cluster damage.

### Q: "How do you handle operators with leader election / multiple replicas?"

**A:** The knowledge model's `expectedSpec.replicas` captures the expected replica count. For PodKill, the `maxPodsAffected` blast radius controls how many pods are killed. For a leader-elected operator with 2 replicas, killing one pod tests leader failover. The knowledge model verifies that the operator still reconciles all resources after the new leader takes over. We don't currently distinguish leader vs. non-leader pods — that could be added via label selectors in the target specification.

### Q: "What's the false positive rate? How often does it report Degraded when the operator is actually fine?"

**A:** The `reconcileTimeout` (default 300s) and `maxReconcileCycles` (default 10) thresholds control this. If a cluster is slow due to resource contention, recovery may take longer than the timeout, producing a Degraded verdict. This is by design — slow recovery is a real operational concern even if the operator "eventually" heals. The thresholds should be tuned per environment. For a heavily loaded staging cluster, increase the timeout. For production-like environments, use the defaults.

### Q: "Can experiments compose or chain? Like 'if ConfigDrift passes, try ConfigDrift + PodKill simultaneously'?"

**A:** Not currently. Each experiment is independent. The `suite` command runs multiple experiments but they don't compose conditionally. For combined fault scenarios, you can create a single experiment that chains injections (e.g., drift a ConfigMap then kill the controller pod) by writing a custom injector. Conditional chaining based on verdicts would require a workflow engine, which is beyond our current scope — we'd rather integrate with Tekton pipelines for that.

### Q: "How does this integrate with OpenShift CI specifically?"

**A:** The container image runs as non-root and produces JUnit XML — both are requirements for OpenShift CI integration. The `suite` command with `--report-dir` generates `suite-results.xml` which OpenShift CI's JUnit harvester picks up. We've documented Tekton Task and Pipeline definitions in `docs/ci-integration-guide.md`. The deployment gating pattern uses Tekton's `runAfter` — if the chaos suite fails, downstream deployment tasks don't execute.

---

## Category 6: Adoption & Roadmap

### Q: "What does 'adopt as a recommended practice' mean concretely?"

**A:** We're proposing a phased approach. Phase 1 (this quarter): pilot with 2-3 operators — write knowledge models, run experiments, validate findings. Phase 2 (after pilot): evaluate results and decide whether to make knowledge models a standard deliverable for new RHOAI operators. If the pilot finds real bugs (as it did with ConfigDrift on inferenceservice-config), the case for standardization makes itself. We're not asking for a mandate today — we're asking for permission to prove the value.

### Q: "What's the effort to onboard a new operator?"

**A:** Writing the knowledge model takes 1-2 hours for someone who knows the operator. It's 30-80 lines of YAML listing managed resources, webhooks, finalizers, and steady-state checks. Running the first experiment takes minutes. The `init` command scaffolds experiment and knowledge model templates.

### Q: "Which operators should we prioritize?"

**A:** We'd value the council's input, but our suggestion is: operators that manage the most resources and have the highest blast radius on failure. After odh-model-controller and kserve, candidates include the dashboard controller, the data science pipelines operator, and the workbenches controller. The prioritization should be driven by which operator failures cause the most customer-visible impact.

### Q: "What's the timeline for the CRD mode?"

**A:** We want to stabilize the experiment API first — at least 2-3 months of usage across multiple operators to validate the schema. After that, the CRD mode is a thin layer: a controller that watches ChaosExperiment CRs and calls the same orchestrator. The orchestrator is already decoupled from the CLI. Estimated 4-6 weeks of implementation after API stabilization.

### Q: "What is the incentive for operator teams to maintain knowledge models?"

**A:** Three things. First, automated regression detection for reconciliation bugs — the ConfigDrift finding is exactly the kind of bug that slips through unit and integration tests and surfaces as a customer escalation months later. Second, the knowledge model itself is valuable documentation: a machine-readable declaration of what the operator manages, useful for onboarding, incident response, and architecture reviews. Third, the chaos suite provides release confidence — an operator team can point to a green chaos suite as evidence their operator handles faults correctly, reducing manual testing burden.

---

## Category 7: Edge Cases & Generalizability

### Q: "What happens if the operator itself is watching the resource you mutate and triggers an unintended reconciliation cascade?"

**A:** That is actually the behavior we want to observe. If the operator watches the resource and reconciles it back, we measure how long that takes and whether the restored state matches the knowledge model. If the mutation triggers a cascade that destabilizes other resources, that is a genuine finding — it means the operator's reconciliation logic has side effects under fault conditions. Our blast radius controls bound the scope, and defer cleanup ensures we restore the original state.

### Q: "How do you handle stateful operators or operators that interact with external systems (e.g., model registries, S3, databases)?"

**A:** Our fault injection targets Kubernetes-native resources only — pods, ConfigMaps, Secrets, RBAC, webhooks, NetworkPolicies. We do not inject faults into external systems. For operators with external dependencies, the SDK middleware mode helps: the ChaosClient can inject errors on API calls the operator makes to the Kubernetes API, simulating delayed or failed responses. For true external dependency testing, combine our tool with infrastructure-level tools like LitmusChaos or Toxiproxy. This is by design — we focus on operator reconciliation correctness, not infrastructure resilience.

### Q: "What about ConfigMap report accumulation? What is the garbage collection strategy?"

**A:** Every report ConfigMap has a TTL annotation (`chaos.opendatahub.io/expires`) set at creation time. The `clean` command removes expired artifacts. For CI pipelines, report ConfigMaps are ephemeral — they live in the CI namespace which is torn down after the pipeline. For long-lived staging environments, run `clean --expired` on a schedule (e.g., a CronJob). The ConfigMaps are small (typically under 4KB each), so storage pressure is unlikely before TTL expiry.

### Q: "What is the testing and quality assurance strategy for the chaos tool itself?"

**A:** The framework has unit tests for every injector, the evaluator, the verdict engine, and all safety layers. Integration tests run full experiment lifecycles. The SDK middleware has its own test suite using controller-runtime's envtest. We also dogfood the tool: we run chaos experiments against a test operator where we know expected verdicts. If the tool reports the wrong verdict against a known-good or known-bad operator, that's a bug in the tool. The rollback and checksum logic have dedicated tests.

### Q: "What is the upgrade and versioning story? If the experiment YAML schema changes, what happens to existing experiments?"

**A:** This is one of the reasons we chose CLI-first over CRD-first. Experiment YAML has a version field, and the CLI validates the version before execution. We commit to backward compatibility within a major version. If we need a breaking schema change, the CLI reports a clear validation error pointing to migration instructions. Since experiments are YAML files in git repositories, they can be migrated with a script. Once we move to a CRD model, we will use Kubernetes API versioning with conversion webhooks for backward compatibility.

### Q: "Have you run this in a production or production-like environment? What were the results beyond the ConfigDrift finding?"

**A:** We have run experiments against staging environments with odh-model-controller and kserve. Beyond the ConfigDrift finding on inferenceservice-config, we validated that PodKill recovery works correctly for odh-model-controller (verdict: Resilient, recovery within 45 seconds). We have not yet run WebhookDisrupt or RBACRevoke experiments against kserve due to the higher danger level — those are planned for the next phase with explicit sign-off from the kserve team. We have not run in production and recommend against it until knowledge models are validated in staging for at least one release cycle.

### Q: "How does this interact with Kubernetes resource quotas and limit ranges?"

**A:** Our fault injections are mutations to existing resources, not creation of new ones — with one exception: the NetworkPartition injector creates a deny-all NetworkPolicy. If the target namespace has a ResourceQuota that limits NetworkPolicies, this injection could fail. The injector catches this error and reports an Inconclusive verdict rather than silently swallowing it. For all other injection types, we modify existing resources, so quotas are not a concern.

### Q: "Why not just use OPA/Gatekeeper for drift detection?"

**A:** OPA/Gatekeeper is an admission controller — it prevents bad configurations from being *applied*. It does not detect drift that happens after admission (e.g., a controller bug that corrupts a ConfigMap it manages, or a resource that is deleted and not recreated). Our tool actively injects faults and verifies the operator's response. They solve different problems: Gatekeeper prevents misconfiguration at write time, we verify recovery at runtime. Both are valuable, and they complement each other.

---

## Category 8: Adversarial & Stress Questions

### Q: "The knowledge model is a single point of fragility. If it's wrong, every verdict is wrong. How do you detect knowledge model rot before it causes false confidence?"

**A:** Three mechanisms. First, `preflight` (cluster mode) validates every declared resource against the live cluster before any experiment — if the operator added or removed resources, preflight catches the mismatch. Second, Inconclusive verdicts are a signal: if experiments that used to pass start returning Inconclusive, the knowledge model likely drifted. Third, we recommend running `preflight` in CI on every operator release — it's a 10-second check that catches staleness. What we don't yet have is automatic detection of *new* resources the operator added that aren't in the model. That's a known gap — the model tests what it declares, but it can't test what it doesn't know about. We're exploring auto-generation from operator RBAC manifests as a future improvement.

### Q: "You say 'defer guarantees cleanup even on crash.' That's not true for SIGKILL or OOM-kill. What happens then?"

**A:** Correct — `defer` does not survive SIGKILL or kernel OOM-kill. We should be precise: `defer` handles panics, context cancellation, and timeouts. For SIGKILL/OOM scenarios, the recovery path is the rollback annotations stored on the mutated resources themselves. These annotations persist independently of the tool's process lifecycle. The `clean` command (or the next experiment invocation) reads these annotations and restores the original state, verifying the SHA-256 checksum before applying. So the guarantee is: `defer` handles graceful failures, annotations handle ungraceful ones. The combination covers both cases, but through different mechanisms.

### Q: "You claim >80% test coverage. Where are the gaps? What's NOT tested?"

**A:** The gaps are primarily in the integration test layer — specifically, the full experiment lifecycle against a real multi-controller operator (we test against a simple test operator). The SDK middleware is well-tested via controller-runtime's envtest, but we haven't tested ChaosClient against every possible controller-runtime version. The injectors are unit-tested with fake Kubernetes clients, which means we don't test against real API server edge cases (e.g., conflict retries under high load, webhook timeout behavior). The coverage number is accurate for line coverage, but line coverage doesn't prove correctness — it proves code was executed. We compensate with the dogfooding approach: running experiments against operators with known-good and known-bad behaviors to validate verdicts.

### Q: "What happens when two teams run experiments against operators that share resources — e.g., both use the same ConfigMap or the same ClusterRoleBinding?"

**A:** This is a real risk. Our distributed lock is per-operator, not per-resource. If operator A and operator B both reference the same ConfigMap, experiments on A and B can run concurrently and potentially conflict on that shared resource. In practice, shared resources in RHOAI are rare — most ConfigMaps and RBAC bindings are scoped to a single operator. For the known shared resources (like cluster-level CRDs), we recommend using `blastRadius.forbiddenResources` to exclude them from mutation. A per-resource lock would be more precise but significantly more complex — we chose simplicity over completeness for the initial design.

### Q: "You're asking us to adopt this as a standard, but you've only tested two operators. That's a small sample. What if odh-model-controller and kserve are unusually well-suited to this approach?"

**A:** Fair challenge. The two operators we've tested represent both ends of the complexity spectrum: odh-model-controller is a single-component operator (6 resources), kserve is a multi-component operator (16 resources, 4 sub-components). The knowledge model abstraction isn't operator-specific — it's a generic description of managed resources, webhooks, finalizers, and steady-state checks. Any operator that follows the controller pattern (watch resources, reconcile to desired state) is testable with this approach. That said, this is exactly why we're asking for a pilot with 2-3 more operators rather than immediate standardization. If the approach doesn't generalize to the dashboard controller or data science pipelines operator, we'll learn that in the pilot and adjust.
