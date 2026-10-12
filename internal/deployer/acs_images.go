package deployer

// TODO(#303): Also include explicit RELATED_IMAGE_* env var overrides in the preload list.
func imagesForConfig(config Config) []string {
	var images []string

	instances, _, _ := config.OperatorInstances()
	for _, instance := range instances {
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
