package helpers

import (
	"os"
	"strings"

	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
)

const (
	defaultMainImageTag = "4.8.4"
)

func LookupMainImageTag(log *logger.Logger) (string, error) {
	log.Info("Looking up main image tag")
	if tag := os.Getenv("MAIN_IMAGE_TAG"); tag != "" {
		log.Dimf("Using MAIN_IMAGE_TAG from environment: %s", tag)
		return tag, nil
	}
	if env.IsInStackroxRepository() {
		tag, err := env.GetStackroxRepositoryTag()
		if err != nil {
			log.Dimf("Error retrieving stackrox repository tag: %v", err)
			return "", err
		}
		log.Dimf("Using stackrox repository tag: %s", tag)
		return tag, nil
	}

	log.Dimf("Using default main image tag %s -- set MAIN_IMAGE_TAG to the desired tag", defaultMainImageTag)
	return defaultMainImageTag, nil
}

func ConvertMainTagToOperatorTag(mainTag string) string {
	if mainTag == "" {
		return ""
	}

	operatorTag := strings.ReplaceAll(mainTag, "-dirty", "")
	operatorTag = strings.ReplaceAll(operatorTag, ".x", ".0")

	return operatorTag
}
