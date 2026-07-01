# Multi-stage Dockerfile for roxie - ACS Deployment Tool
# Produces a compact container image with roxie and all required dependencies
# Supports multi-architecture builds (amd64, arm64)

# Stage 1: Build roxie binary
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:1.25@sha256:2830e4bd1c394ed506c00a9abbb4d00445e2e72e8ef4e3cd51e0da0db66dee12 AS builder

# Build arguments for cross-compilation
# These are automatically provided by Docker buildx
ARG TARGETOS
ARG TARGETARCH

WORKDIR /build
USER root
ENV GOTOOLCHAIN=auto

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build roxie binary with version info and cross-compilation support
ARG ROXIE_VERSION
ARG BUILD_DATE
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    ROXIE_VERSION=${ROXIE_VERSION} \
    BUILD_DATE=${BUILD_DATE} \
    make build

# Download gcloud SDK in builder stage to avoid UBI filesystem restrictions
# Latest version including checksums can be found at:
#   https://docs.cloud.google.com/sdk/docs/install-sdk#linux
#
# Unfortunately Googles release pipelines currently do not properly support versioned, checksum-protected downloads,
#
# THE PROBLEM
#
#   The page https://docs.cloud.google.com/sdk/docs/install-sdk#linux references download links which are
#   unversioned, which is not suitable for CI. For these unversioned links the page contains checksums.
#
#   The SDK can also be downloaded throught versioned links, which is suitable for CI usage. However, these
#   versioned links are not referenced in the page and -- more importantly -- the checksums of both
#   files (versioned and unversioned) are *not* the same. They differ in the filename contained in the gzip header.
#
# THE WORKAROUND
#
#   I have downloaded both files, versioned and unversioned, together with the latest checksum
#   from the download page for the unversioned file. Then I have decompressed both files, verified
#   that both archives are bytewise identical and then I have compted the sha256 of the versioned file
#   and inserted it here.
#
#   Example:
#
#   ❯ curl -sLfO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz
#   ❯ curl -sLfO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-562.0.0-linux-x86_64.tar.gz
#   ❯ UNVERSIONED_CHECKSUM=38bd4f203392354fa7cc5514ee38ea02bb808aa5f1f7e00257806abf782dde38
#   ❯ gzip -dk google-cloud-cli-562.0.0-linux-x86_64.tar.gz; gzip -dk google-cloud-cli-linux-x86_64.tar.gz
#   ❯ echo "${UNVERSIONED_CHECKSUM} google-cloud-cli-linux-x86_64.tar.gz" | sha256sum -c -
#   google-cloud-cli-linux-x86_64.tar.gz: OK
#   ❯ cmp google-cloud-cli-562.0.0-linux-x86_64.tar google-cloud-cli-linux-x86_64.tar
#   ❯ sha256 google-cloud-cli-562.0.0-linux-x86_64.tar.gz
#   SHA256 (google-cloud-cli-562.0.0-linux-x86_64.tar.gz) = 016a4b1702f8c97b585f9ae12c6182762758c17ef5302cb8561c7f6be5cc9af3
#
ARG GCLOUD_VERSION=562.0.0
ARG GCLOUD_ARM64_SHA256=a9ebaa0f4020ea0487c2c935af3d4566d1b4a1ccae685c6b7141211fc96424ee
ARG GCLOUD_AMD64_SHA256=016a4b1702f8c97b585f9ae12c6182762758c17ef5302cb8561c7f6be5cc9af3
RUN ARCH=${TARGETARCH:-amd64} && \
    if [ "${ARCH}" = "amd64" ]; then \
        GCLOUD_ARCH="x86_64"; \
        GCLOUD_SHA256=${GCLOUD_AMD64_SHA256}; \
    elif [ "${ARCH}" = "arm64" ]; then \
        GCLOUD_ARCH="arm"; \
        GCLOUD_SHA256=${GCLOUD_ARM64_SHA256}; \
    else \
        echo "ERROR: Unsupported architecture: ${ARCH}"; exit 1; \
    fi && \
    filename="google-cloud-cli-${GCLOUD_VERSION}-linux-${GCLOUD_ARCH}.tar.gz" && \
    url="https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/${filename}" && \
    echo "Downloading gcloud SDK from: ${url}" && \
    curl -o "/tmp/${filename}" -fsSL "${url}" && \
    echo "${GCLOUD_SHA256} /tmp/${filename}" | sha256sum -c - && \
    tar -xz -C /tmp -f "/tmp/${filename}"

# Stage 2: Runtime image based on Red Hat UBI Minimal
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:ae09ecc3d754bc1726cbda3e2599cc7839e09fe1cc547ce173cf669b645be3cc

# Architecture detection for multi-arch builds
ARG TARGETARCH

LABEL maintainer="StackRox" \
      description="roxie - Advanced Cluster Security deployment tool with all dependencies" \
      io.k8s.description="Deploy and manage Red Hat Advanced Cluster Security on Kubernetes clusters" \
      io.k8s.display-name="roxie ACS Deployment Tool"

# Install required tools via microdnf
# Note: UBI minimal comes with curl pre-installed, which is sufficient for our needs
RUN microdnf install -y \
    # Core utilities
    tar \
    gzip \
    unzip \
    ca-certificates \
    # Python (required for gcloud SDK)
    python3 \
    # Clean up
    && microdnf clean all \
    && rm -rf /var/cache/yum

# Install kubectl - architecture-aware
# Checksums can be found at
#   https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl.sha256.
ARG KUBECTL_VERSION=v1.35.3
ARG KUBECTL_ARM64_SHA256=6f0cd088a82dde5d5807122056069e2fac4ed447cc518efc055547ae46525f14
ARG KUBECTL_AMD64_SHA256=fd31c7d7129260e608f6faf92d5984c3267ad0b5ead3bced2fe125686e286ad6
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing kubectl for ${ARCH}" && \
    if [ "${ARCH}" = "arm64" ]; then \
        KUBECTL_SHA256=${KUBECTL_ARM64_SHA256}; \
    elif [ "${ARCH}" = "amd64" ]; then \
        KUBECTL_SHA256=${KUBECTL_AMD64_SHA256}; \
    else \
        echo "ERROR: Unsupported architecture: ${ARCH}"; exit 1; \
    fi && \
    url="https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl" && \
    echo "Downloading kubectl from: ${url}" && \
    curl -fsSLo /usr/local/bin/kubectl "${url}" && \
    echo "${KUBECTL_SHA256}  /usr/local/bin/kubectl" | sha256sum -c - && \
    chmod +x /usr/local/bin/kubectl

# Install roxctl - architecture-aware
# The mirror has architecture-specific binaries: 'roxctl' (amd64) and 'roxctl-arm64'
ARG ROXCTL_VERSION=4.10.0
ARG ROXCTL_ARM64_SHA256=a3951413d56671e46413009d31004d984e9c77c7520f35c8f062f5bd4e4f8212
ARG ROXCTL_AMD64_SHA256=5db647b14569465866c0162522e83393ebf02f671f4556b1b3ed551b9f8433bc
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing roxctl for ${ARCH}" && \
    if [ "${ARCH}" = "arm64" ]; then \
        ROXCTL_BINARY="roxctl-arm64"; \
        ROXCTL_SHA256=${ROXCTL_ARM64_SHA256}; \
    elif [ "${ARCH}" = "amd64" ]; then \
        ROXCTL_BINARY="roxctl"; \
        ROXCTL_SHA256=${ROXCTL_AMD64_SHA256}; \
    else \
        echo "ERROR: Unsupported architecture: ${ARCH}"; exit 1; \
    fi && \
    url="https://mirror.openshift.com/pub/rhacs/assets/${ROXCTL_VERSION}/bin/Linux/${ROXCTL_BINARY}" && \
    echo "Downloading from: ${url}" && \
    curl -fsSLo /usr/local/bin/roxctl "${url}" && \
    echo "${ROXCTL_SHA256}  /usr/local/bin/roxctl" | sha256sum -c - && \
    chmod +x /usr/local/bin/roxctl


# Install common kubectl credential plugins for cloud provider authentication
# This enables kubectl to work with GKE, EKS, AKS, and OpenShift clusters
# without requiring users to manage different auth plugins

# 1. Google Cloud (GKE) - gke-gcloud-auth-plugin
# Copy gcloud SDK from builder stage (extracted there to avoid UBI filesystem restrictions)
COPY --from=builder /tmp/google-cloud-sdk /opt/google-cloud-sdk
RUN ln -s /opt/google-cloud-sdk/bin/gcloud /usr/local/bin/gcloud && \
    /opt/google-cloud-sdk/bin/gcloud components install gke-gcloud-auth-plugin --quiet && \
    ln -s /opt/google-cloud-sdk/bin/gke-gcloud-auth-plugin /usr/local/bin/gke-gcloud-auth-plugin

# 2. AWS (EKS) - aws-iam-authenticator
# Using GitHub releases for latest version (AWS S3 bucket has outdated versions)
ARG AWS_IAM_AUTH_VERSION=0.7.12
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing aws-iam-authenticator v${AWS_IAM_AUTH_VERSION} for ${ARCH}" && \
    curl -fsSLo /usr/local/bin/aws-iam-authenticator \
    "https://github.com/kubernetes-sigs/aws-iam-authenticator/releases/download/v${AWS_IAM_AUTH_VERSION}/aws-iam-authenticator_${AWS_IAM_AUTH_VERSION}_linux_${ARCH}" && \
    chmod +x /usr/local/bin/aws-iam-authenticator

# 3. Azure (AKS) - kubelogin
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing kubelogin (Azure) for ${ARCH}" && \
    KUBELOGIN_VERSION="v0.2.16" && \
    curl -fsSL "https://github.com/Azure/kubelogin/releases/download/${KUBELOGIN_VERSION}/kubelogin-linux-${ARCH}.zip" \
    -o /tmp/kubelogin.zip && \
    unzip -q /tmp/kubelogin.zip -d /tmp && \
    mv /tmp/bin/linux_${ARCH}/kubelogin /usr/local/bin/kubelogin && \
    chmod +x /usr/local/bin/kubelogin && \
    rm -rf /tmp/kubelogin.zip /tmp/bin

# Copy roxie binary and scripts from builder
COPY --from=builder /build/roxie /usr/local/bin/roxie
COPY scripts/roxcurl /usr/local/bin/roxcurl
RUN chmod +x /usr/local/bin/roxcurl

# Create non-root user with / as home directory for simplified volume mounting
# This allows users to mount credentials directly at their standard paths:
#   -v ~/.kube:/.kube:ro instead of -v ~/.kube:/home/roxie/.kube:ro
RUN useradd -r -u 1000 -d / -s /bin/bash roxie \
    && mkdir -p /.kube /.roxie /.config /.aws /.azure \
    && chown -R roxie:roxie /.kube /.roxie /.config /.aws /.azure

# Set working directory
WORKDIR /workspace

# Switch to non-root user (users can override with --user root if needed)
USER roxie

# Set environment variables
ENV HOME=/ \
    RUNNING_IN_ROXIE_CONTAINER=true \
    KUBECONFIG=/kubeconfig \
    PATH=/usr/local/bin:$PATH

# Display version information on container start
ENTRYPOINT ["/usr/local/bin/roxie"]
CMD ["--help"]
