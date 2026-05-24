# Getting Started Example

Deploys PingFederate on a local Docker Desktop cluster using the Ping Identity
getting-started server profiles. Ingress is handled by nginx with self-signed TLS
via cert-manager.

## Prerequisites

- Docker Desktop with Kubernetes enabled
- [nginx ingress controller](https://kubernetes.github.io/ingress-nginx/deploy/#docker-desktop)
- [cert-manager](https://cert-manager.io/docs/installation/)
- A [Ping Identity DevOps account](https://docs.pingidentity.com/devops/devopsPrograms/devopsRegistration.html)
- The operator installed (`make install && make docker-desktop` from the repo root)

## Steps

### 1. Install the cert-manager ClusterIssuer

```bash
kubectl apply -f cert-manager.yaml
```

### 2. Fill in your DevOps credentials

Edit `environment.yaml` and replace the placeholder values in the `devops-secret`:

```bash
# Generate the base64 values
echo -n "user@example.com" | base64   # -> PING_IDENTITY_DEVOPS_USER
echo -n "your-devops-key"  | base64   # -> PING_IDENTITY_DEVOPS_KEY
```

### 3. Apply the environment

```bash
kubectl apply -f environment.yaml
```

### 4. Watch the rollout

```bash
kubectl get pingenvironments -n pingone -w
kubectl get pods -n pingone -w
```

### 5. Access PingFederate

Add to `/etc/hosts`:
```
127.0.0.1  pf.localhost pf-admin.localhost
```

Then open:
- Engine: `https://pf.localhost`
- Admin console: `https://pf-admin.localhost` (default credentials: `Administrator` / `2FederateM0re`)

### Tear down

```bash
kubectl delete -f environment.yaml
kubectl delete -f cert-manager.yaml
```
