package helpers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/ocihelper"
	"github.com/stackrox/roxie/internal/stackroxversions"
)

func LookupMainImageTag(ctx context.Context, log *logger.Logger) (string, error) {
	log.Dim("Checking if main image tag is defined in the environment")
	if tag := os.Getenv("MAIN_IMAGE_TAG"); tag != "" {
		log.Infof("Using MAIN_IMAGE_TAG from environment: %s", tag)
		return tag, nil
	}
	log.Dim("Checking if current working directory is checkout of stackrox/stackrox repository")
	if env.IsInStackroxRepository(log) {
		tag, err := env.GetStackroxRepositoryTag(log)
		if err != nil {
			log.Dimf("Error retrieving stackrox repository tag: %v", err)
			return "", err
		}
		log.Infof("Using stackrox repository tag: %s", tag)
		return tag, nil
	}

	log.Warning("No tag specified and no MAIN_IMAGE_TAG found in the environment, looking up latest release tag on registry")
	log.Warning("To use a different tag, use `--tag` or set the MAIN_IMAGE_TAG environment variable")
	log.Warning("Alternatively, execute roxie from within the stackrox repository, in which case the currently checked out stackrox tag will be used")

	log.Dim("Checking what the latest released version tag is")
	latestTag, err := LookupLatestTag(ctx, log)
	if err != nil {
		return "", fmt.Errorf("looking up latest release tag: %w", err)
	}
	log.Infof("Using latest released tag %v", latestTag)

	return latestTag, nil
}

// Computes the latest image tag for a pullable, released main image.
func LookupLatestTag(ctx context.Context, log *logger.Logger) (string, error) {
	const atMost = 5

	tags, err := stackroxversions.LookupLatestReleaseTagsViaGitHub(ctx, atMost)
	if err != nil {
		return "", fmt.Errorf("looking up latest release tags: %w", err)
	}

	// Verify we have a pullable main image.
	for _, tag := range tags {
		mainImage := fmt.Sprintf("%s/main:%s", constants.DefaultRegistry, tag)
		if err := ocihelper.VerifyImageExistence(ctx, log, mainImage); err != nil {
			var te *transport.Error
			if errors.As(err, &te) && te.StatusCode == http.StatusNotFound {
				continue
			}
			return "", fmt.Errorf("verifying image %s: %w", mainImage, err)
		}
		return tag, nil
	}

	return "", fmt.Errorf("failed to verify main image existence for tags %s", strings.Join(tags, ", "))
}

func ConvertMainTagToOperatorTag(mainTag string) string {
	if mainTag == "" {
		return ""
	}

	operatorTag := strings.ReplaceAll(mainTag, "-dirty", "")
	operatorTag = strings.ReplaceAll(operatorTag, ".x", ".0")

	return operatorTag
}
