package deployer

var (
	internalCentralEndpoint = "central.acs-central.svc:443"

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
)
