# Speaker Notes — ODH Platform Chaos Architects Council

## Delivery Tips (from architect reviews)

1. **Lead with the ConfigDrift finding.** Tease it on slide 2, deliver the payoff on slide 17. The architecture matters, but the real finding is what sells.
2. **Anticipate the "maintenance burden" objection and own it.** Say: "The operator team maintains it, as part of their definition of done. A knowledge model is 30-80 lines of YAML — less than a Dockerfile. If you can't describe what your operator reconciles, you have a bigger problem."
3. **Speed through the non-goals slide.** 60 seconds max. The audience appreciates scope awareness but dwelling here kills momentum before the ask.
4. **On the verdict engine slide, walk through the ConfigDrift scenario.** "Pre-check passed — ConfigMap existed. Post-check failed — data still corrupted after 300s. Verdict: Failed. The controller doesn't watch for external drift."
5. **Have the actual knowledge model YAML open in a terminal.** If someone asks "what does a knowledge model look like?", show `knowledge/odh-model-controller.yaml` (79 lines) or `knowledge/kserve.yaml` (~200 lines) live.
6. **If running long by slide 14, compress slides 18 and 20 to one sentence each.** Failure modes and non-goals can be deferred to Q&A.

**Target timing:** 28-30 minutes of content, 15-20 minutes Q&A. Budget extra time for Q&A — architects will debate the adoption path. The softened asks (pilot first, advisory mode) reduce friction but expect questions about what "standardization" means concretely after the pilot.
7. **Be honest about limitations proactively.** If someone asks about defer guarantees, say "defer handles panics and cancellation; SIGKILL/OOM is covered by rollback annotations — two mechanisms, not one." Owning the nuance before being challenged builds more credibility than being caught overstating.

---

## Slide 1: Title

> "Good morning/afternoon. I'm here to present ODH Platform Chaos — a chaos engineering framework we've built specifically for testing OpenDataHub operator resilience. This is not a generic chaos tool. It's purpose-built to answer one question: after a fault, does the operator semantically restore everything it manages?"

---

## Slide 2: The Problem We Solve

> "Traditional chaos tools like LitmusChaos and Chaos Mesh are excellent at infrastructure-level testing — kill a pod, partition the network, see if things come back. But they answer a surface-level question: does the pod restart?"

> "We ask a harder question. After a fault, does the operator restore ALL its managed resources — not just the deployment, but every ConfigMap, every webhook registration, every RBAC binding, every owner reference? A pod can restart perfectly and still leave the system broken."

> "This distinction matters. I'll show you a real example later where we found exactly this gap in odh-model-controller."

---

## Slide 3: Why This Matters for RHOAI

> "Let me ground this in our operators. odh-model-controller manages 6 resources, 7 webhooks, and 3 finalizers. kserve is even more complex — 16 managed resources, 12 webhooks, 4 finalizers across 4 sub-components."

> "These counts come directly from our knowledge models, verified against deployed manifests."

> "Here's a concrete risk: the `inferenceservice-config` ConfigMap. If this ConfigMap drifts — someone edits it manually, a script overwrites it, a controller bug corrupts it — every single InferenceService deployment on the cluster breaks. No pod restarts. No alerts fire. It's a silent failure. We'll come back to this."

---

## Slide 4: Who Uses This and When

> "Now that you know the problem, who cares about it? Four personas, four usage modes."

> "Operator developers use the SDK middleware in their integration tests — every PR. QE engineers run the CLI suite against staging before release. SREs use it for incident preparation. And CI pipelines run it automatically to gate promotions."

> "The common thread: every persona needs to verify semantic healing, not just pod restarts."

---

## Slide 5: Why Build, Not Extend?

> "The obvious next question: why build something new? The answer is in this table. The top four rows are things neither LitmusChaos nor Chaos Mesh does: semantic reconciliation verification, knowledge-model-driven testing, SDK middleware for unit/integration tests, and fuzz testing without a cluster."

> "These aren't features you can bolt onto an infrastructure chaos tool. They require a fundamentally different architecture — one that understands what an operator should reconcile, not just what Kubernetes resources exist."

> "We're not competing with LitmusChaos. We're complementary. They test the platform. We test the operator."

---

## Slide 6: Architecture Overview

> "Now let me show you how we built this. The orchestrator is the heart of the system — let me walk through the diagram top to bottom."

> "A CLI built on cobra drives the orchestrator. The orchestrator coordinates six components: injectors execute faults, the observer checks health, the evaluator determines verdicts, the safety layer enforces guardrails, the reporter produces output, and the knowledge model provides the oracle."

> "Everything is injected via OrchestratorConfig — no globals, no singletons. The framework itself has unit and integration tests across all packages."

---

## Slide 7: Experiment Lifecycle

> "That was the static view. Let me show you how these components work together at runtime."

> "Every experiment follows this state machine. We establish a baseline with SteadyStatePre, inject the fault, observe the operator's response, re-check health with SteadyStatePost, and evaluate the result."

> "Notice the 'Interruptible' column. Before injection, we can abort cleanly — no mutations have happened. After injection, an interrupt triggers cleanup. The critical invariant: cleanup runs in defer — on context cancellation, timeout, or panic. For harder failures like SIGKILL or OOM, rollback annotations on the resources survive and the clean command recovers them. An interrupted experiment always rolls back — through one mechanism or another."

---

## Slide 8: Injection Registry

> "Let me zoom into the injection system. Every injector implements this interface — two methods."

> "Validate checks preconditions, Inject executes the fault and returns a CleanupFunc. The CleanupFunc reverses the fault. The orchestrator calls it in defer, guaranteeing rollback even if the experiment crashes. This is how we ensure the tool never makes things worse."

> "Why an interface? Each injection type has fundamentally different Kubernetes API interactions. Killing pods is nothing like modifying webhook configurations. A common interface enables the registry pattern while allowing each injector to do type-specific validation."

---

## Slide 9: Seven Injection Types

> "Seven injection types covering five categories of operator failure. Pod lifecycle, network, configuration, control plane, and lifecycle hooks."

> "The danger levels guide safety gates. Low-danger injections like PodKill run without extra approval. High-danger injections like WebhookDisrupt and RBACRevoke require explicit `allowDangerous: true` in the experiment YAML. This forces the experiment author to acknowledge the risk."

> "Seven fault types, some of them high-danger. So how do we make sure this tool doesn't cause more damage than the fault it's testing?"

---

## Slide 10: Safety Architecture

> "Six layers of defense, each catching what the previous one might miss. I'll highlight three that come up most in practice."

> "Layer 1: blast radius — namespace whitelist, pod count limits, forbidden resources. Whitelists, not blacklists — if you didn't explicitly allow a namespace, it's denied."

> "Layer 3: distributed locking via Kubernetes Leases. One lease per operator name. 15-minute auto-expiry prevents deadlocks from crashed experiments."

> "Layer 5: rollback data with SHA-256 checksums. Before we mutate any resource, we serialize the original state, compute a checksum, and store both in an annotation. On cleanup, we verify the checksum before restoring."

---

## Slide 11: Security, Authorization & Audit

> "Security is delegated to Kubernetes RBAC. The chaos tool runs as a regular Kubernetes client. If your ServiceAccount doesn't have delete on pods, you can't run a PodKill experiment. Period."

> "Why no custom auth layer? Because Kubernetes RBAC already solves this. Adding a second authorization layer creates a false sense of security and doubles the surface area for misconfiguration."

> "Audit trail: every experiment produces a report ConfigMap with the experiment name, timestamp, verdict, injection details, and recovery metrics. Kubernetes audit logs capture which ServiceAccount created it, so you have full provenance."

---

## Slide 12: Knowledge Model YAML

> "The engine above can inject faults and verify rollback. But how does it know what 'correct' looks like for a specific operator? That's what the knowledge model answers."

> "This is what a knowledge model looks like. Typically 30-80 lines of YAML per operator. This one describes odh-model-controller: its managed resources, webhooks, finalizers, steady-state checks, and recovery expectations."

> "The recovery section is critical: reconcileTimeout of 300 seconds and a maximum of 10 reconcile cycles. These are the thresholds that determine whether a recovery is 'Resilient' or 'Degraded'."

[If asked: "Have the actual YAML open in a terminal to show the full file"]

---

## Slide 13: What the Knowledge Model Enables

> "Without the knowledge model, chaos testing is blind — kill a pod, check if it restarts. With it, we can verify the full reconciliation: all 6 managed resources, not just the deployment. Owner references. Expected spec values. Webhook re-registration. Recovery time and cycle count."

> "Teams maintain one YAML per operator, versioned alongside the operator code. Updates are needed roughly once per release cycle when an operator adds or removes managed resources."

> [If time is short, skip OLM and expressiveness limits — save for Q&A]

> "For OLM-managed operators, the knowledge model references resources the operator creates — not resources OLM creates from the CSV. This avoids conflicts with OLM's own reconciliation."

---

## Slide 14: Verdict Engine

> "That's what we verify. Now how do we decide the outcome?"

> "Four verdicts, not two. Inconclusive means we couldn't establish a baseline — the system was already broken. Failed means it didn't recover. Degraded means it recovered but with issues — slow recovery, partial reconciliation, or thrashing."

> "Degraded is the most interesting verdict. It says 'the operator healed, but not well enough.' That's actionable for optimization without blocking releases."

> [Walk through with ConfigDrift example]: "For our inferenceservice-config experiment: pre-check passed — the ConfigMap existed and was healthy. Post-check failed — the data was still corrupted after 300 seconds. Verdict: Failed. The controller doesn't watch for external drift on this resource."

---

## Slide 15: Three Usage Modes

> "Now let's talk about the three ways you actually run these experiments."

> "Three modes, one knowledge model. CLI experiments run against a live cluster with full lifecycle. SDK middleware injects faults at the API level in integration tests — no cluster needed. Fuzz testing generates random fault configurations for unit tests."

> "Let me zoom into the SDK mode."

---

## Slide 16: SDK Middleware

> "ChaosClient wraps any controller-runtime client. It implements the full `client.Client` interface — drop-in replacement. Every Kubernetes API call goes through MaybeInject, which decides whether to inject a fault based on the configuration."

> "Performance overhead is negligible — one map lookup and a random number check per call. No reflection, no deep copies."

> "Why middleware, not mocks? Mocks require knowing the exact call sequence upfront. Middleware injects faults probabilistically into real code paths, testing actual error handling — including error handling you forgot to test."

---

## Slide 17: Concrete Example

> [THIS IS THE KEY SLIDE — deliver with conviction]

> "Here's a real experiment we ran. ConfigDrift on the inferenceservice-config ConfigMap. We corrupted the config key with garbage data and watched what happened."

> "What we expected: odh-model-controller would detect the drift and restore the ConfigMap to its correct state within 300 seconds."

> "What actually happened: the controller did NOT reconcile this ConfigMap. It creates it during installation but never watches for drift. Any manual edit, any accidental overwrite, any script that touches this ConfigMap silently breaks every InferenceService deployment on the cluster. No alerts. No pod restarts. Just silent failure."

> "This finding drove a reconciler fix upstream. This is what semantic chaos testing finds that pod-kill testing doesn't."

---

## Slide 18: Tool Failure Modes

> "That finding is exactly what this tool is for. But what if the tool itself fails? We designed for that too."

> "Five scenarios, each with a mitigation. The most important design principle: the tool must never make things worse than the fault it injected."

> "If the tool crashes mid-injection, defer cleanup runs — on panic, timeout, or cancellation. For harder failures like SIGKILL or OOM-kill where defer doesn't execute, rollback annotations persist on the resources — the clean command restores them on next invocation. Two recovery paths, covering both graceful and ungraceful failures."

---

## Slide 19: CI Integration

> "This matters because the tool was designed from day one to be a CI citizen, not a manual exercise."

> "Standard Unix exit codes. Resilient is exit 0, everything else is non-zero. Suite generates JUnit XML automatically. Reports also go to Kubernetes ConfigMaps, queryable by label selector."

> "Container image is distroless, non-root, multi-arch. Tekton Tasks and GitHub Actions workflows are documented in the CI integration guide."

---

## Slide 20: Non-Goals

> [Speed through this — 60 seconds max]

> "Five deliberate non-goals, each with a reason. No CRD controller — we want the experiment API to stabilize before committing to a CRD schema. No iptables injection — requires privileged containers, conflicts with our non-root security model. No multi-cluster — fundamentally different coordination problem."

---

## Slide 21: Roadmap & Ask

> "Core framework, SDK, CLI, and CI integration are done. What's next: knowledge models for all RHOAI operators, a CRD mode once the API stabilizes, and integration into the OpenShift CI release gating pipeline."

> "Three asks to this council:"

> "First: adopt knowledge models as a recommended practice. We're proposing a pilot with 2-3 operators this quarter — not mandating it for everyone today. Let the results speak. If the pilot finds real bugs, standardization follows naturally."

> "Second: run chaos suites in advisory mode — integrate into CI as informational reports for 2 release cycles before we gate on results. This lets teams see the value without the risk of false-positive blocking."

> "Third: help us prioritize. Beyond odh-model-controller and kserve, which operators should we model next? We'd value your input on where the highest risk lies."

---

## Slide 22: Summary

> "ODH Platform Chaos tests what matters — semantic reconciliation correctness, not just pod restarts. Six architectural decisions drive the design: knowledge-driven, safety-first, interface-driven, three usage modes, CI-native, and RBAC-delegated security."

> "One question to take away: for each RHOAI operator, can we prove that after any of these 7 fault types, the operator restores all managed resources to their correct state within the expected time? That's what this tool answers."

> "Thank you. I'm happy to take questions."
