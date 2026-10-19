# Existing cluster deployment

Run these commands on 192.168.200.5 as w-ubuntu, from:
`/mnt/sdb/准备迁移/Projects/GolandProjects/experiments/e-commerce`.

## Cluster access

```bash
export KUBECONFIG="$HOME/.kube/remote-config"
kubectl get nodes
```

The default kubeconfig has an invalid current context. Use the explicit
remote-config above; do not replace the existing kubeconfig.

## Scope

This overlay deploys the API, PostgreSQL, Redis and RabbitMQ to `ecommerce`.
Persistent volumes use `local-path`. Preserve the existing PVCs and Secrets.
Elasticsearch is not deployed; product search is unavailable until it is added.
The API never changes the database schema at startup. Apply reviewed Atlas
migrations separately before rolling out an application version that needs them;
see [the database migration guide](../docs/database-migrations.md).

## Build and update

The current release tag is `fde25f7-testfix-20261012`, containing the tested order fixes in `64bfeb1` and MQ fixes in `a6152eb`.
Use a new tag for each release; update the overlay image tag accordingly.

```bash
docker build --build-arg REGISTRY_PREFIX=harbor.internal/dockerhub/   --label org.opencontainers.image.version=fde25f7-testfix-20261012   -t harbor.internal/e-commerce/app:fde25f7-testfix-20261012 .
docker push harbor.internal/e-commerce/app:fde25f7-testfix-20261012
kubectl diff -k k8s/overlays/harbor-local
# diff exit code 1 means differences; exit code >1 means an error.
kubectl apply -k k8s/overlays/harbor-local
kubectl -n ecommerce rollout status deployment/ecommerce-api --timeout=180s
kubectl -n ecommerce get pods,pvc
```

Existing Secrets are managed outside Kustomize. Do not replace their values
with repository templates or commit secret.local.yaml files.

## Access and verify

Ingress uses NodePort 30080 (HTTP), not port 80 directly on the nodes.
The existing hostname resolves to 192.168.20.21 on the Ubuntu host.

```bash
curl --noproxy '*' -f http://ecommerce.k8s.internal:30080/health
curl --noproxy '*' -f http://ecommerce.k8s.internal:30080/ready
# Without hostname resolution:
curl --noproxy '*' -f -H 'Host: ecommerce.k8s.internal'   http://192.168.20.21:30080/ready
```

Swagger: `http://ecommerce.k8s.internal:30080/swagger`.
Readiness must report database, redis and rabbitmq as `ok`.

## Roll back this release

The previous image is `harbor.internal/e-commerce/app:fde25f7`.
Change the API image tag in this overlay back to `fde25f7`, review
`kubectl diff -k k8s/overlays/harbor-local`, then apply the overlay.
This release changes the API image only; its ConfigMap and database schema
are unchanged. `kubectl rollout undo` restores the Deployment only,
so also update the overlay tag to keep Git and the cluster consistent.

API deployment snapshot before this update: `tmp/k8s-testfix-20261012/deployment-before.yaml`.

See [interface and integration tests](../docs/testing.md) for business verification.

## Automatic product verification

The product pipeline builds an image, updates the test API Deployment and then
runs HTTP business acceptance in one serialized job. The namespace-scoped
credential is stored as a GitLab file variable for ecommerce-test.
CI deploys IMAGE_REPO:CI_COMMIT_SHORT_SHA; the image tag in this static overlay
is the manual baseline. Review/update that tag before manually applying the
whole overlay so that a manual apply does not replace the CI release.
Infrastructure, ConfigMaps, Secrets and PVCs are not changed by the CI job.
