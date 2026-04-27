# kueue Custom Experiments

This page provides templates for writing custom chaos experiments targeting kueue.


## kueue-controller-manager

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kueue-controller-manager-custom
spec:
  target:
    operator: kueue
    component: kueue-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kueue-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app=kueue-controller-manager
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
