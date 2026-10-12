#!/usr/bin/env bash
set -euo pipefail
: "${KUBECONFIG:?Provide the test cluster kubeconfig as a GitLab file variable}"
: "${IMAGE_REPO:?Missing release image repository}"
: "${CI_COMMIT_SHORT_SHA:?Missing release tag}"
: "${API_TEST_BASE_URL:?Missing test API origin}"
test -f "$KUBECONFIG"
namespace="${K8S_NAMESPACE:-ecommerce}"
deployment="${K8S_DEPLOYMENT:-ecommerce-api}"
container="${K8S_CONTAINER:-ecommerce-api}"
release_image="$IMAGE_REPO:$CI_COMMIT_SHORT_SHA"
mkdir -p test-results
collect_diagnostics() {
  result=$?
  if [ "$result" -ne 0 ]; then
    kubectl -n "$namespace" get deployment "$deployment" -o wide >test-results/deployment.txt 2>&1 || true
    kubectl -n "$namespace" get pods -l "app=$deployment" -o wide >test-results/pods.txt 2>&1 || true
    kubectl -n "$namespace" logs "deployment/$deployment" --tail=100 >test-results/app.log 2>&1 || true
  fi
}
trap collect_diagnostics EXIT
kubectl -n "$namespace" set image "deployment/$deployment" "$container=$release_image"
kubectl -n "$namespace" rollout status "deployment/$deployment" --timeout=180s
actual_image="$(kubectl -n "$namespace" get deployment "$deployment" -o "jsonpath={.spec.template.spec.containers[?(@.name==\"$container\")].image}")"
if [ "$actual_image" != "$release_image" ]; then
  echo "Deployment image does not match this pipeline's release" >&2
  exit 1
fi
printf '%s\n' "$release_image" >test-results/release-image.txt
# One resource-group job serializes deployment and acceptance.
API_TEST_ALLOW_WRITES=1 go test ./tests/acceptance -v -count=1 -timeout=3m \
  2>&1 | tee test-results/acceptance.log
