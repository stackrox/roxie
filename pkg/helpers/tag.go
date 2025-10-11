package helpers

import (
	"os"
	"strings"
)

const (
	defaultMainImageTag = "4.8.2"
)

func LookupMainImageTag() string {
	if tag := os.Getenv("MAIN_IMAGE_TAG"); tag != "" {
		return tag
	}
	return defaultMainImageTag
}

func ConvertMainTagToOperatorTag(mainTag string) string {
	if mainTag == "" {
		return ""
	}

	operatorTag := strings.ReplaceAll(mainTag, "-dirty", "")
	operatorTag = "v" + strings.ReplaceAll(operatorTag, ".x", ".0")

	return operatorTag
}
