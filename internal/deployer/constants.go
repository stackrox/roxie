package deployer

import "fmt"

var (
	centralCrName        = "stackrox-central-services"
	securedClusterCrName = "stackrox-secured-cluster-services"

	centralDbPVCSizeTiny  = "10Gi"
	centralDbPVCSizeSmall = "30Gi"

	centralResourcesTiny = map[string]interface{}{
		"requests": map[string]string{
			"memory": "600Mi",
			"cpu":    "300m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1",
		},
	}

	centralResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "1Gi",
			"cpu":    "500m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1",
		},
	}

	centralDbResourcesTiny = map[string]interface{}{
		"requests": map[string]string{
			"memory": "600Mi",
			"cpu":    "300m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1",
		},
	}

	centralDbResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "1Gi",
			"cpu":    "500m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1",
		},
	}

	centralScannerResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "500Mi",
			"cpu":    "500m",
		},
		"limits": map[string]string{
			"memory": "2500Mi",
			"cpu":    "2000m",
		},
	}

	centralScannerDbResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "200m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
	}

	centralScannerV4DbResourcesTiny = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "300m",
		},
		"limits": map[string]string{
			"memory": "2000Mi",
			"cpu":    "1000m",
		},
	}

	centralScannerV4DbResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "400m",
		},
		"limits": map[string]string{
			"memory": "2000Mi",
			"cpu":    "1000m",
		},
	}

	centralScannerV4IndexerResourcesTiny = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "300m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "2000m",
		},
	}

	centralScannerV4IndexerResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "400m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "2000m",
		},
	}

	centralScannerV4MatcherResourcesTiny = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "300m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
	}

	centralScannerV4MatcherResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "400m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
	}

	// Secured Cluster

	securedClusterSensorResourcesTiny = map[string]interface{}{
		"requests": map[string]string{
			"memory": "500Mi",
			"cpu":    "300m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
	}

	securedClusterSensorResourcesSmall = map[string]interface{}{
		"requests": map[string]string{
			"memory": "500Mi",
			"cpu":    "500m",
		},
		"limits": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
	}

	// Medium resources - midpoint between small and default (with reasonable rounding)

	centralResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
		"limits": map[string]string{
			"memory": "5Gi",
			"cpu":    "2500m",
		},
	}

	centralDbResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "4Gi",
			"cpu":    "2000m",
		},
		"limits": map[string]string{
			"memory": "9Gi",
			"cpu":    "4500m",
		},
	}

	centralScannerResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "1Gi",
			"cpu":    "750m",
		},
		"limits": map[string]string{
			"memory": "3Gi",
			"cpu":    "2000m",
		},
	}

	centralScannerDbResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "200m",
		},
		"limits": map[string]string{
			"memory": "3Gi",
			"cpu":    "1500m",
		},
	}

	centralScannerV4DbResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "2Gi",
			"cpu":    "700m",
		},
		"limits": map[string]string{
			"memory": "5Gi",
			"cpu":    "2500m",
		},
	}

	centralScannerV4IndexerResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "512Mi",
			"cpu":    "1000m",
		},
		"limits": map[string]string{
			"memory": "2500Mi",
			"cpu":    "3000m",
		},
	}

	centralScannerV4MatcherResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "1Gi",
			"cpu":    "450m",
		},
		"limits": map[string]string{
			"memory": "2500Mi",
			"cpu":    "1000m",
		},
	}

	securedClusterSensorResourcesMedium = map[string]interface{}{
		"requests": map[string]string{
			"memory": "2Gi",
			"cpu":    "1000m",
		},
		"limits": map[string]string{
			"memory": "5Gi",
			"cpu":    "2500m",
		},
	}

	// CI resources - based on deploy/common/ci-values.yaml from stackrox repo.

	centralDbResourcesCI = map[string]interface{}{
		"requests": map[string]string{
			"memory": "1Gi",
			"cpu":    "1",
		},
		"limits": map[string]string{
			"memory": "16Gi",
			"cpu":    "8",
		},
	}

	centralScannerV4DbResourcesCI = map[string]interface{}{
		"requests": map[string]string{
			"cpu": "500m",
		},
	}

	centralScannerV4IndexerResourcesCI = map[string]interface{}{
		"requests": map[string]string{
			"cpu": "400m",
		},
	}

	centralScannerV4MatcherResourcesCI = map[string]interface{}{
		"requests": map[string]string{
			"cpu": "400m",
		},
	}

	securedClusterSensorResourcesCI = map[string]interface{}{
		"requests": map[string]string{
			"memory": "500Mi",
			"cpu":    "500m",
		},
	}
)

func internalCentralEndpoint(namespace string) string {
	return fmt.Sprintf("central.%s.svc:443", namespace)
}
