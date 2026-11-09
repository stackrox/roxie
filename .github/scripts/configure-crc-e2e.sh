#!/usr/bin/env bash
set -euo pipefail

# Only for the disposable CRC cluster created by e2e-tests-crc.yml. Do not run
# against a developer's CRC: CVO overrides disable normal operator management.
if [[ "${GITHUB_ACTIONS:-}" != "true" || "${SKIP_OLM_TESTS:-}" != "true" ]]; then
  echo "This script requires GitHub Actions and SKIP_OLM_TESTS=true" >&2
  exit 1
fi
server=$(oc config view --minify -o jsonpath='{.clusters[0].cluster.server}')
if [[ "$server" != "https://api.crc.testing:6443" ]]; then
  echo "Refusing to configure a non-CRC cluster: $server" >&2
  exit 1
fi

wait_for_empty() {
  local namespace=$1 resource=$2
  shift 2
  local resources deadline=$((SECONDS + 300))
  while true; do
    # CRC's API can briefly reset connections while the cluster reconciles.
    # A failed list is not an empty list: retry it within the same deadline.
    if resources=$(oc --request-timeout=15s get "$resource" -n "$namespace" "$@" -o name); then
      if [[ -z "$resources" ]]; then
        return
      fi
    else
      resources="API request failed"
      echo "Retrying list of $resource in $namespace after an API error" >&2
    fi
    if (( SECONDS >= deadline )); then
      echo "Timed out waiting for $resource in $namespace: $resources" >&2
      return 1
    fi
    sleep 5
  done
}

echo "Node memory allocation before trimming OpenShift"
oc describe nodes

# Use the supported removal settings first so their operators clean up their
# operands. ACS pulls from Quay, and these tests never use the OpenShift UI,
# internal registry, sample builds, or OLM. Do not alter ACS/Scanner V4 settings.
# https://docs.okd.io/4.22/web_console/disabling-web-console.html
oc patch consoles.operator.openshift.io cluster --type=merge \
  --patch='{"spec":{"managementState":"Removed"}}'
oc patch configs.imageregistry.operator.openshift.io cluster --type=merge \
  --patch='{"spec":{"managementState":"Removed"}}'
oc patch configs.samples.operator.openshift.io cluster --type=merge \
  --patch='{"spec":{"managementState":"Removed"}}'
oc patch operatorhub.config.openshift.io cluster --type=merge \
  --patch='{"spec":{"disableAllDefaultSources":true}}'
oc patch networks.operator.openshift.io cluster --type=merge \
  --patch='{"spec":{"disableNetworkDiagnostics":true}}'

wait_for_empty openshift-console deployments
wait_for_empty openshift-console pods
wait_for_empty openshift-image-registry deployments image-registry --ignore-not-found
wait_for_empty openshift-image-registry pods -l docker-registry=default
wait_for_empty openshift-marketplace catalogsources
wait_for_empty openshift-marketplace pods -l olm.catalogSource
wait_for_empty openshift-network-diagnostics pods
oc wait configs.samples.operator.openshift.io/cluster \
  --for=jsonpath='{.status.managementState}'=Removed --timeout=5m

# Wait for the migrator operator to stabilize before stopping its controllers.
# This CI cluster is never upgraded and its fixed CRC VM needs no Machine API
# provisioning. Keep the machine approver and machine-config stack running.
oc wait clusteroperator/kube-storage-version-migrator \
  --for=condition=Available=True --timeout=5m
oc wait clusteroperator/kube-storage-version-migrator \
  --for=condition=Progressing=False --timeout=5m

# Enabled capabilities cannot be disabled after installation. For this
# short-lived cluster only, prevent CVO from restoring unused operators, using
# the same per-object override mechanism CRC uses to disable monitoring/cloud
# operators. Preserve all existing CRC overrides. Core cluster operators remain
# managed; we do not scale CVO itself down.
deployments='[
  ["openshift-operator-lifecycle-manager", "olm-operator"],
  ["openshift-operator-lifecycle-manager", "catalog-operator"],
  ["openshift-operator-lifecycle-manager", "package-server-manager"],
  ["openshift-operator-lifecycle-manager", "packageserver"],
  ["openshift-marketplace", "marketplace-operator"],
  ["openshift-console-operator", "console-operator"],
  ["openshift-cluster-samples-operator", "cluster-samples-operator"],
  ["openshift-image-registry", "cluster-image-registry-operator"],
  ["openshift-kube-storage-version-migrator-operator", "kube-storage-version-migrator-operator"],
  ["openshift-kube-storage-version-migrator", "migrator"],
  ["openshift-machine-api", "machine-api-operator"],
  ["openshift-machine-api", "control-plane-machine-set-operator"]
]'
patch=$(oc get clusterversion version -o json | jq --argjson deployments "$deployments" '
  . as $version
  |
  ($deployments | map({group: "apps", kind: "Deployment", namespace: .[0], name: .[1], unmanaged: true}))
  + [{group: "apiregistration.k8s.io", kind: "APIService", namespace: "", name: "v1.packages.operators.coreos.com", unmanaged: true}]
  | . as $disabled
  | reduce $disabled[] as $item (($version.spec.overrides // []);
    map(select([.group, .kind, .namespace, .name] != [$item.group, $item.kind, $item.namespace, $item.name])) + [$item])
  | {spec: {overrides: .}}
')
oc patch clusterversion version --type=merge --patch="$patch"
generation=$(oc get clusterversion version -o jsonpath='{.metadata.generation}')
oc wait clusterversion/version \
  --for="jsonpath={.status.observedGeneration}=$generation" --timeout=5m

# Stop OLM before its managed packageserver, then stop the remaining unused
# operators. Wait for each deployment to terminate before stopping its children.
while IFS=$'\t' read -r namespace name; do
  if [[ "$namespace" == "openshift-operator-lifecycle-manager" && "$name" == "packageserver" ]]; then
    # Its managers have stopped. Remove the registration before stopping the
    # server so discovery/namespace deletion never depend on a dead endpoint.
    oc delete apiservice v1.packages.operators.coreos.com --ignore-not-found
  fi
  oc scale deployment "$name" -n "$namespace" --replicas=0
  selector=$(oc get deployment "$name" -n "$namespace" -o json | jq -er '
    .spec.selector.matchLabels | to_entries | map("\(.key)=\(.value)") | join(",")
    | if length == 0 then error("Deployment has no label selector") else . end
  ')
  wait_for_empty "$namespace" pods -l "$selector"
done < <(jq -r '.[] | @tsv' <<< "$deployments")

echo "Node memory allocation after trimming OpenShift"
oc describe nodes
oc get --raw=/readyz
