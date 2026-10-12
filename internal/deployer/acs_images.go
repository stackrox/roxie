package deployer

func imagesForConfig(config Config) []string {
	var images []string

	for _, instance := range config.OperatorInstances() {
		images = append(images,
			constructOperandImageFromInstance(instance, "main"),
			constructOperandImageFromInstance(instance, "central-db"),
			constructOperandImageFromInstance(instance, "scanner-v4-db"),
			constructOperandImageFromInstance(instance, "scanner-v4"),
			instance.OperatorImage(),
			instance.BundleImage(),
		)
	}

	return images
}
