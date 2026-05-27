# pingone-operator Helm Chart

This chart deploys the PingOne Kubernetes Operator and its required RBAC resources.

## Install

```bash
helm upgrade --install pingone-operator ./charts/pingone-operator \
  --namespace pingone-system \
  --create-namespace
```

## Install from OCI

```bash
helm upgrade --install pingone-operator oci://<registry>/<repo>/pingone-operator \
  --version <chart-version> \
  --namespace pingone-system \
  --create-namespace
```

## Key values

- `image.repository`: Operator image repository
- `image.tag`: Operator image tag
- `watchNamespace.currentNamespaceOnly`: Watch only release namespace (`true` by default)
- `watchNamespace.value`: Explicit namespace to watch (takes precedence when set)
