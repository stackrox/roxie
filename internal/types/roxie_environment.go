package types

import (
	"iter"
	"reflect"
	"strings"
)

// RoxieEnvironment is the environment used during runtime. It includes another field,
// which shall not be stored on the cluster, since it is a local path.
type RoxieEnvironment struct {
	APIEndpoint      string `yaml:"API_ENDPOINT,omitempty"`
	RoxAdminPassword string `yaml:"ROX_ADMIN_PASSWORD,omitempty"`
	RoxBaseURL       string `yaml:"ROX_BASE_URL,omitempty"`
	RoxEndpoint      string `yaml:"ROX_ENDPOINT,omitempty"`
	RoxUsername      string `yaml:"ROX_USERNAME,omitempty"`
	RoxCaCertFile    string `yaml:"ROX_CA_CERT_FILE,omitempty"`
}

// We use this type for standard YAML marshaling.
type roxieEnvironmentStd RoxieEnvironment

// MarshalYAML skips local-only fields (ROX_CA_CERT_FILE).
func (r RoxieEnvironment) MarshalYAML() (any, error) {
	shallowCopy := roxieEnvironmentStd(r)
	shallowCopy.RoxCaCertFile = "" // Filter out local fields.
	return shallowCopy, nil
}

// Export() returns an iterator, which produces key,val pairs for the roxie environment.
// This includes local-only fields unlike YAML serialization, which only produces those
// values which shall actually be persisted.
func (r RoxieEnvironment) Export() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		v := reflect.ValueOf(r)
		for _, f := range reflect.VisibleFields(v.Type()) {
			if tag, ok := f.Tag.Lookup("yaml"); ok && tag != ",inline" {
				val := v.FieldByIndex(f.Index).String()
				if val == "" {
					continue
				}
				name, _, _ := strings.Cut(tag, ",")
				if !yield(name, val) {
					return
				}
			}
		}
	}
}
