# Local Image Support for Kind Clusters

**Date**: 2026-01-20
**Status**: Implemented (with variations from original design)
**Author**: Design session with user

> **Note**: This document reflects the initial design. The final implementation differs in several areas:
> - Image detection checks both branding organizations (not just ROX_PRODUCT_BRANDING)
> - Localhost paths were removed (only quay.io paths used)
> - Added CSV patching for operator deployments
> - Image list includes collector, scanner-v4, and stackrox-operator
> - See git history (commits bc376ad through 9d8a54b) for implementation evolution

## Overview

Enable roxie to automatically use locally-built container images when deploying to kind clusters, eliminating the need to push images to quay.io during local development.

## Problem Statement

Current workflow for testing local ACS builds:
1. Build stackrox locally (images go to podman)
2. Push images to quay.io
3. Deploy via roxie from quay.io

This upload/download cycle is slow and unnecessary for local kind cluster testing.

## Goals

- **Primary**: Seamless local development workflow - build locally, deploy locally with zero extra steps
- **Zero configuration**: Automatic detection and transparent fallback
- **Fast**: Eliminate network round-trip to quay.io
- **Reliable**: Fail fast on errors, don't silently fall back

## Non-Goals

- Support for non-kind local clusters (minikube, k3d, etc.) - can be added later if needed
- Support for remote kind clusters
- Image building or tagging - assumes images already exist in podman

## Design

### High-Level Architecture

**Core Strategy**: Automatic detection and transparent fallback

- When deploying to a kind cluster, roxie automatically detects if images exist locally in podman
- If local images exist, roxie loads them into kind before deployment
- If local images don't exist, roxie falls back to pulling from quay.io (existing behavior)
- Zero configuration required - it "just works"

**Detection Chain**:
1. Is this a kind cluster? - Check if current KUBECONFIG context points to a kind cluster
2. Are images available locally? - Check podman for images using branding-aware paths
3. Load if both true - Use `kind load docker-image` to transfer images from podman to kind

### Branding Support

**ROX_PRODUCT_BRANDING Environment Variable**:
- Read `ROX_PRODUCT_BRANDING` environment variable
- Default to `RHACS_BRANDING` if not set
- Branding determines registry organization:
  - `RHACS_BRANDING` → `quay.io/rhacs-eng`
  - `STACKROX_BRANDING` → `quay.io/stackrox-io`

**Image Naming**:
- Local stackrox builds create dual-tagged images:
  - `localhost/stackrox/<image>:<tag>`
  - `quay.io/<branding-org>/<image>:<tag>`
- Both tags point to the same image ID in podman

### Image Inventory and Detection

**Images Required for Deployment** (as implemented):

Main images (7):
- `main:<tag>`
- `scanner:<tag>`
- `scanner-db:<tag>`
- `scanner-v4:<tag>`
- `scanner-v4-db:<tag>`
- `central-db:<tag>`
- `collector:<tag>`

Operator images (2):
- `stackrox-operator:<operatorTag>`
- `stackrox-operator-bundle:v<operatorTag>`

Total: 9 images

Note: Original design included scanner-v4-indexer, scanner-v4-matcher, and stackrox-operator-index, but these were removed/consolidated during implementation.

**Detection Algorithm** (as implemented):

For each required image:
```
Function: checkLocalImage(imageName, tag)
  1. Determine primary org based on ROX_PRODUCT_BRANDING (defaults to "rhacs-eng")
  2. Determine fallback org (the other branding org)
  3. Check: podman image exists quay.io/<primary-org>/<imageName>:<tag>
  4. If not found, check: podman image exists quay.io/<fallback-org>/<imageName>:<tag>
  5. Return: (imageRef, true, nil) if found, ("", false, nil) if not found
```

**Key Implementation Differences**:
- ✅ Checks BOTH branding orgs to handle images that only exist in one org (e.g., collector)
- ✅ Only uses quay.io paths (localhost paths removed in commit bc376ad)
- ✅ Returns idiomatic (value, bool, error) tuple
- **Implementation**: Use `podman image exists <ref>` command (exit code 0 = exists, 1 = doesn't exist)

### Kind Cluster Detection

**Detection Method**:
```
Function: isKindCluster()
  1. Get current context name from kubectl/KUBECONFIG
  2. Check if context name starts with "kind-" prefix
  3. Return: true if kind cluster detected, false otherwise
```

**Cluster Name Extraction**:
- Context name format: `kind-<cluster-name>`
- Extract cluster name for use in `kind load docker-image -n <cluster-name>`
- Example: context `kind-acs` → cluster name `acs`

### Image Loading to Kind

**Loading Mechanism**:
```
Function: loadImageToKind(imageRef, clusterName)
  1. Execute: kind load docker-image <imageRef> -n <clusterName>
  2. kind automatically detects podman via DOCKER_HOST or default socket
  3. Log progress: "Loading <imageRef> into kind cluster <clusterName>"
  4. On failure: Abort deployment with clear error (loading failures are fatal)
```

**Parallel Loading**:
- Load multiple images concurrently (4 workers, matching existing image verification parallelism)
- Show progress to user: "Loading 8 images into kind cluster..."
- Estimated time: ~10-30 seconds depending on image sizes

### Deployment Workflow Integration

**Modified Deployment Flow**:

```
1. Parse flags and validate configuration
2. Resolve image tags (MAIN_IMAGE_TAG, operator tags)
3. Connect to cluster
4. [NEW] Detect if kind cluster
5. [NEW] If kind: check for local images and load them
6. Verify credentials (skip if using all local images)
7. Create namespaces
8. Create image pull secrets (skip if using all local images)
9. Deploy via operator or helm (existing behavior)
```

**Optimization When All Images Are Local**:
- Skip quay.io credential verification (allows completely offline development)
- Skip creating image pull secrets (local images don't need authentication)

**User Feedback**:
- "Detected kind cluster 'acs'"
- "Found 8/8 images locally in podman"
- "Loading images into kind cluster..." (with progress)
- "All images loaded, skipping credential verification"

### Error Handling and Edge Cases

**Partial Local Images**:
- **Scenario**: Some images exist locally, others don't (e.g., 5/8 found)
- **Behavior**: Load available local images to kind, let remaining images be pulled from quay.io by kubelet
- **Credentials**: Still create image pull secrets (needed for remote images)
- **User feedback**: "Found 5/8 images locally, loading these. Remaining images will be pulled from quay.io"

**Image Loading Failures**:
- **Scenario**: `kind load docker-image` fails for a specific image
- **Behavior**: **Fail deployment immediately** with clear error message
- **Rationale**: User expects local images to be used; failure indicates real problem (wrong podman socket, permissions, corrupted image, etc.)
- **Error message**: "Failed to load local image <ref> into kind cluster: <error>. Aborting deployment."

**Tag Mismatch**:
- **Scenario**: `MAIN_IMAGE_TAG=4.10.0` but local images are tagged `4.10.x-827-g6ab85ec46b`
- **Behavior**: No match found, fall back to quay.io
- **Future enhancement**: Could support fuzzy matching or tag aliases

**Non-Kind Clusters**:
- **Scenario**: Deploying to OpenShift, GKE, EKS, regular k8s, etc.
- **Behavior**: Skip kind detection and image loading entirely, use existing quay.io flow
- **Impact**: Zero behavior change for non-kind deployments (backward compatible)

**podman Not Available**:
- **Scenario**: roxie running in environment without podman
- **Behavior**: Skip local image detection (podman commands fail gracefully), fall back to quay.io
- **Rationale**: Allows roxie to work in containerized environments or systems with only docker

**Override/Disable**:
- **Optional**: Add environment variable `ROXIE_SKIP_LOCAL_IMAGES=true` to force quay.io behavior even on kind
- **Use case**: Debugging, testing quay.io pulls on kind, working around issues

## Implementation Plan

### New Components

**1. Package: `internal/localimages`**
- `branding.go`: Handle ROX_PRODUCT_BRANDING environment variable, map to registry paths
- `detection.go`: Check for local images in podman
- `loading.go`: Load images into kind cluster

**2. Package: `internal/cluster`**
- `kind.go`: Detect kind clusters, extract cluster name from context

### Modified Components

**1. `internal/helpers/tag.go`**
- No changes needed - existing tag resolution works as-is

**2. `internal/dockerauth/dockerauth.go`**
- Modify credential verification to be skippable when all images are local

**3. `cmd/deploy.go`**
- Integrate local image detection and loading into deployment flow
- Add step between cluster connection and credential verification

**4. `internal/deployer/operator.go`**
- Potentially skip pull secret creation if all images are local

**5. `internal/deployer/deploy_via_helm.go`**
- Image verification may need adjustment to skip local images

### Testing Strategy

**Manual Testing**:
1. Build stackrox locally with RHACS branding
2. Verify images in podman: `podman images | grep stackrox`
3. Deploy to kind with roxie: should auto-detect and load images
4. Verify deployment uses local images (check pod image IDs)
5. Test with STACKROX branding
6. Test partial local images (delete some images)
7. Test non-kind cluster (should fall back to quay.io)
8. Test with `ROXIE_SKIP_LOCAL_IMAGES=true`

**Edge Case Testing**:
- Missing podman (should gracefully fall back)
- Kind cluster but no local images (should use quay.io)
- Loading failure (should fail deployment with clear error)
- Mixed brandings (local RHACS, remote STACKROX - should work)

## Future Enhancements

**Potential additions if needed**:
1. Support for other local cluster types (k3d, minikube, microk8s)
2. Fuzzy tag matching (4.10.0 matches 4.10.x-*)
3. Local registry mode for clusters that can't use direct loading
4. Verbose logging mode showing exact podman/kind commands
5. Image pre-loading cache to skip re-loading unchanged images

## Success Criteria

- Developer can build stackrox locally and deploy to kind without pushing to quay.io
- Zero configuration required - works automatically when conditions are met
- Backward compatible - no behavior change for non-kind deployments
- Fast - eliminates network round-trip, loads complete in <30 seconds
- Reliable - clear errors on failures, no silent fallback when local images are detected

## Non-Functional Requirements

- **Performance**: Image loading should complete within 30 seconds for typical ACS deployment (8 images)
- **Compatibility**: Must work with both podman and docker (via compatible sockets)
- **Backward Compatibility**: No breaking changes to existing roxie functionality
- **Maintainability**: Code should be well-structured and easy to extend for other cluster types later
