# kserve Custom Experiments

This page provides templates for writing custom chaos experiments targeting kserve.


## kserve-controller-manager

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kserve-controller-manager-custom
spec:
  target:
    operator: kserve
    component: kserve-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kserve-controller-manager
        namespace: kserve
        conditionType: Available
    timeout: "60s"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app=kserve-controller-manager
    ttl: "300s"
  hypothesis:
    description: >-
      Describe the expected behavior after fault injection.
    recoveryTimeout: 120s
```

## llmisvc-controller-manager

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: llmisvc-controller-manager-custom
spec:
  target:
    operator: kserve
    component: llmisvc-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: llmisvc-controller-manager
        namespace: kserve
        conditionType: Available
    timeout: "60s"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app=llmisvc-controller-manager
    ttl: "300s"
  hypothesis:
    description: >-
      Describe the expected behavior after fault injection.
    recoveryTimeout: 120s
```

## kserve-localmodel-controller-manager

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kserve-localmodel-controller-manager-custom
spec:
  target:
    operator: kserve
    component: kserve-localmodel-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kserve-localmodel-controller-manager
        namespace: kserve
        conditionType: Available
    timeout: "60s"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app=kserve-localmodel-controller-manager
    ttl: "300s"
  hypothesis:
    description: >-
      Describe the expected behavior after fault injection.
    recoveryTimeout: 120s
```

## kserve-localmodelnode-agent

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kserve-localmodelnode-agent-custom
spec:
  target:
    operator: kserve
    component: kserve-localmodelnode-agent
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: apps/v1
        kind: DaemonSet
        name: kserve-localmodelnode-agent
        namespace: kserve
    timeout: "60s"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app=kserve-localmodelnode-agent
    ttl: "300s"
  hypothesis:
    description: >-
      Describe the expected behavior after fault injection.
    recoveryTimeout: 120s
```


## Running Custom Experiments

1. Save your experiment YAML to a file
2. Run: `chaos-cli run --experiment <file>`
3. Check results: `chaos-cli results --latest`

<!-- custom-start: examples -->
<!-- custom-end: examples -->
