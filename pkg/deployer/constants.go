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
			"cpu":    "400m",
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
)
