package deployer

func imagesForConfig(config Config) []string {
	var images []string

	for _, instance := range config.OperatorInstances() {
		for _, imageName := range relatedImages {
			images = append(images, constructOperandImageFromInstance(instance, imageName))
		}
		images = append(images,
			instance.OperatorImage(),
			instance.BundleImage(),
		)
	}

	return images
}
