# opendatahub-operator Custom Experiments

This page provides templates for writing custom chaos experiments targeting opendatahub-operator.


## opendatahub-operator

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: opendatahub-operator-custom
spec:
  target:
    operator: opendatahub-operator
    component: opendatahub-operator
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: opendatahub-operator-controller-manager
        namespace: opendatahub-operator-system
        conditionType: Available
    timeout: "60s"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app=opendatahub-operator
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
