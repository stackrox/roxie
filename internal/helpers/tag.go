package helpers

import (
	"os"
	"strings"

	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
)

const (
	// TODO(#91): Is the plan to keep bumping this on new ACS releases?
	defaultMainImageTag = "4.9.2"
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

	log.Warningf("No MAIN_IMAGE_TAG found in the environment, using default main image tag %s for deployment", defaultMainImageTag)
	log.Warning("To use a different tag, set the MAIN_IMAGE_TAG environment variable")
	log.Warning("Alternatively, execute roxie from within the stackrox repository, in which case the currently checked out stackrox tag will be used")

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
