# Tests

This directory contains tests for the roxie project.

## Structure

- `e2e/` - End-to-end integration tests that require a Kubernetes cluster
- `testhelpers/` - Shared test utilities and helpers

## Running Tests

### Unit Tests

Run all unit tests:
```bash
make test
```

Run tests with coverage:
```bash
make test-coverage
```

Run tests in short mode (faster):
```bash
make test-short
```

### E2E Tests

E2E tests require:
- A running Kubernetes cluster
- `kubectl` configured with the target cluster context
- The roxie binary built (`make build`)

Run E2E tests:
```bash
make test-e2e
```

Or directly with go:
```bash
# Note: -parallel=1 ensures tests run sequentially to avoid conflicts
go test -v -tags=e2e -timeout=30m -parallel=1 ./tests/e2e/...
```

### Environment Variables for E2E Tests

- `MAIN_IMAGE_TAG` - ACS image tag to use (default: "4.8.2")
- `SKIP_OPERATOR_TESTS` - Skip operator-based tests if set
- `SKIP_OLM_TESTS` - Skip OLM-specific tests if set (useful when OLM is not available)
- `SKIP_IMAGE_VERIFICATION` - Skip image verification if set to "true"

Example:
```bash
MAIN_IMAGE_TAG=4.9.0 make test-e2e
```

### Running OLM Switching Tests

The OLM switching tests verify that roxie can properly switch between OLM and non-OLM operator deployment modes. These tests:
- Require OLM installed in the cluster
- Require sufficient cluster resources
- Test all switching scenarios: OLM↔non-OLM, version upgrades, multi-component deployments

Run OLM tests:
```bash
# Run all OLM tests (they run sequentially via -parallel=1)
go test -v -tags=e2e -timeout=120m -parallel=1 ./tests/e2e/ -run TestOLM

# Run specific OLM test
go test -v -tags=e2e -timeout=60m -parallel=1 ./tests/e2e/ -run TestOLMToNonOLMSwitch
```

Skip OLM tests if OLM is not available:
```bash
SKIP_OLM_TESTS=1 go test -v -tags=e2e -timeout=30m -parallel=1 ./tests/e2e/
```

## Test Organization

### Unit Tests

Unit tests are co-located with the code they test:
- `pkg/dockerauth/dockerauth_test.go` - Docker authentication tests
- `pkg/imagecache/imagecache_test.go` - Image cache tests
- `pkg/helpers/helpers_test.go` - Helper function tests

### E2E Tests

E2E tests use build tags to prevent them from running during normal unit tests:

```go
// +build e2e

package e2e
```

This ensures they only run when explicitly requested with `-tags=e2e`.

## Writing Tests

### Unit Test Example

```go
package mypackage

import (
    "testing"
    "github.com/stackrox/roxie/tests/testhelpers"
)

func TestMyFunction(t *testing.T) {
    log, capture := testhelpers.CreateTestLogger(t)

    // Your test code here
    result := MyFunction(log)

    // Assertions
    testhelpers.AssertNoError(t, result)
    testhelpers.AssertContains(t, capture.Stdout.String(), "expected output")
}
```

### E2E Test Example

```go
// +build e2e

package e2e

import (
    "os"
    "testing"
    "time"
)

// Note: All e2e tests run sequentially via the -parallel=1 flag in the Makefile
// to prevent conflicts when modifying shared cluster resources.

func TestDeployment(t *testing.T) {
    if os.Getenv("SKIP_OPERATOR_TESTS") != "" {
        t.Skip("Operator tests disabled")
    }

    runCommand(t, 30*time.Minute, nil,
        roxieBinary, "deploy", "central")

    verifyNamespaceExists(t, "acs-central")
}
```

## Test Helpers

The `testhelpers` package provides utilities:

- `CreateTestLogger(t)` - Logger with captured output
- `AssertContains(t, haystack, needle)` - String contains assertion
- `AssertEqual(t, expected, actual)` - Equality assertion
- `AssertNoError(t, err)` - Error nil assertion
- `AssertError(t, err)` - Error not nil assertion

## CI Integration

The Makefile provides targets suitable for CI:

```bash
# Run all checks
make check

# Run all tests
make test test-e2e

# Full CI workflow
make all
```

## Coverage

View coverage report in browser:
```bash
make test-coverage
# Opens coverage.html in your browser
```

## Troubleshooting

### E2E Tests Timing Out

Increase the timeout:
```bash
go test -v -tags=e2e -timeout=60m ./tests/e2e/...
```

### Tests Can't Find Kubernetes Context

Ensure kubectl is configured:
```bash
kubectl config current-context
```

### Binary Not Found

Build the binary first:
```bash
make build
```
