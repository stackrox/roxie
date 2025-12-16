# Multi-stage Dockerfile for roxie - ACS Deployment Tool
# Produces a compact container image with roxie and all required dependencies
# Supports multi-architecture builds (amd64, arm64)

# Stage 1: Build roxie binary
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.25-alpine AS builder

# Build arguments for cross-compilation
# These are automatically provided by Docker buildx
ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build roxie binary with version info and cross-compilation support
ARG VERSION=0.1
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN echo "Building for ${TARGETOS}/${TARGETARCH}" && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o roxie \
    ./cmd

# Stage 2: Runtime image based on Red Hat UBI Minimal
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Architecture detection for multi-arch builds
ARG TARGETARCH

LABEL maintainer="StackRox" \
      description="roxie - Advanced Cluster Security deployment tool with all dependencies" \
      io.k8s.description="Deploy and manage Red Hat Advanced Cluster Security on Kubernetes clusters" \
      io.k8s.display-name="roxie ACS Deployment Tool"

# Install required tools via microdnf
# kubectl, helm are available in RHEL repos
# Note: UBI minimal comes with curl pre-installed, which is sufficient for our needs
RUN microdnf install -y \
    # Core utilities
    tar \
    gzip \
    unzip \
    ca-certificates \
    # Container tools
    podman \
    # Python (required for gcloud SDK)
    python3 \
    # Clean up
    && microdnf clean all \
    && rm -rf /var/cache/yum

# Install kubectl - architecture-aware
ARG KUBECTL_VERSION=v1.29.0
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing kubectl for ${ARCH}" && \
    curl -sLo /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl" \
    && chmod +x /usr/local/bin/kubectl

# Install helm - architecture-aware
ARG HELM_VERSION=v3.14.0
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing helm for ${ARCH}" && \
    curl -sL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz" \
    | tar xz -C /tmp \
    && mv /tmp/linux-${ARCH}/helm /usr/local/bin/helm \
    && chmod +x /usr/local/bin/helm \
    && rm -rf /tmp/linux-${ARCH}

# Install roxctl - architecture-aware
# The mirror has architecture-specific binaries: 'roxctl' (amd64) and 'roxctl-arm64'
# Override with --build-arg ROXCTL_VERSION=4.x.x for specific versions
ARG ROXCTL_VERSION=latest
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing roxctl for ${ARCH}" && \
    if [ "${ARCH}" = "arm64" ]; then \
        ROXCTL_BINARY="roxctl-arm64"; \
    elif [ "${ARCH}" = "amd64" ]; then \
        ROXCTL_BINARY="roxctl"; \
    else \
        echo "ERROR: Unsupported architecture: ${ARCH}"; exit 1; \
    fi && \
    ROXCTL_PATH=$([ "${ROXCTL_VERSION}" = "latest" ] && echo "latest" || echo "${ROXCTL_VERSION}") && \
    ROXCTL_URL="https://mirror.openshift.com/pub/rhacs/assets/${ROXCTL_PATH}/bin/Linux/${ROXCTL_BINARY}" && \
    echo "Downloading from: ${ROXCTL_URL}" && \
    curl -fsSLo /usr/local/bin/roxctl "${ROXCTL_URL}" && \
    chmod +x /usr/local/bin/roxctl && \
    echo "roxctl installed successfully for ${ARCH}"

# Install podman (required for extracting operator bundles)
# fuse-overlayfs provides better performance but vfs driver is more compatible
RUN microdnf install -y podman fuse-overlayfs \
    && microdnf clean all

# Install common kubectl credential plugins for cloud provider authentication
# This enables kubectl to work with GKE, EKS, AKS, and OpenShift clusters
# without requiring users to manage different auth plugins

# 1. Google Cloud (GKE) - gke-gcloud-auth-plugin
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing gcloud SDK and gke-gcloud-auth-plugin for ${ARCH}" && \
    # Map Docker arch names to gcloud package names
    if [ "${ARCH}" = "amd64" ]; then \
        GCLOUD_ARCH="x86_64"; \
    elif [ "${ARCH}" = "arm64" ]; then \
        GCLOUD_ARCH="arm"; \
    else \
        echo "ERROR: Unsupported architecture: ${ARCH}"; exit 1; \
    fi && \
    # Download and install gcloud SDK
    curl -sL "https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-${GCLOUD_ARCH}.tar.gz" \
    | tar xz -C /opt && \
    # Install gke-gcloud-auth-plugin
    /opt/google-cloud-sdk/bin/gcloud components install gke-gcloud-auth-plugin --quiet && \
    # Create symlinks in PATH
    ln -s /opt/google-cloud-sdk/bin/gcloud /usr/local/bin/gcloud && \
    ln -s /opt/google-cloud-sdk/bin/gke-gcloud-auth-plugin /usr/local/bin/gke-gcloud-auth-plugin

# 2. AWS (EKS) - aws-iam-authenticator
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing aws-iam-authenticator for ${ARCH}" && \
    curl -sLo /usr/local/bin/aws-iam-authenticator \
    "https://amazon-eks.s3.us-west-2.amazonaws.com/1.30.0/2024-05-12/bin/linux/${ARCH}/aws-iam-authenticator" && \
    chmod +x /usr/local/bin/aws-iam-authenticator

# 3. Azure (AKS) - kubelogin
RUN ARCH=${TARGETARCH:-amd64} && \
    echo "Installing kubelogin (Azure) for ${ARCH}" && \
    KUBELOGIN_VERSION="v0.1.4" && \
    curl -sL "https://github.com/Azure/kubelogin/releases/download/${KUBELOGIN_VERSION}/kubelogin-linux-${ARCH}.zip" \
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
    && mkdir -p /.kube /.roxie /.local/share/containers /.config /.aws /.azure \
    && chown -R roxie:roxie /.kube /.roxie /.local /.config /.aws /.azure

# Configure podman for rootless operation inside container
# This is critical for roxie's operator bundle extraction functionality
# Using VFS storage driver for maximum compatibility in containerized environments
RUN mkdir -p /etc/containers && \
    echo 'unqualified-search-registries = ["docker.io", "quay.io"]' > /etc/containers/registries.conf && \
    echo '[storage]' > /etc/containers/storage.conf && \
    echo 'driver = "vfs"' >> /etc/containers/storage.conf && \
    echo 'runroot = "/tmp/containers/storage"' >> /etc/containers/storage.conf && \
    echo 'graphroot = "/.local/share/containers/storage"' >> /etc/containers/storage.conf && \
    chmod 644 /etc/containers/storage.conf /etc/containers/registries.conf

# Set working directory
WORKDIR /workspace

# Switch to non-root user (users can override with --user root if needed)
USER roxie

# Set environment variables
ENV HOME=/ \
    KUBECONFIG=/kubeconfig \
    PATH=/usr/local/bin:$PATH

# Display version information on container start
ENTRYPOINT ["/usr/local/bin/roxie"]
CMD ["--help"]
