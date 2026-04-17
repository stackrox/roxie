package deployer

import (
	"testing"
)

func TestSetCentralEndpoint(t *testing.T) {
	tests := []struct {
		name                         string
		input                        string
		expectedCentralEndpoint      string
		expectedUserProvidedEndpoint string
	}{
		{
			name:                         "plain host:port",
			input:                        "10.0.0.1:443",
			expectedCentralEndpoint:      "10.0.0.1:443",
			expectedUserProvidedEndpoint: "10.0.0.1:443",
		},
		{
			name:                         "strips https prefix",
			input:                        "https://10.0.0.1:443",
			expectedCentralEndpoint:      "10.0.0.1:443",
			expectedUserProvidedEndpoint: "10.0.0.1:443",
		},
		{
			name:                         "hostname with port",
			input:                        "central.example.com:443",
			expectedCentralEndpoint:      "central.example.com:443",
			expectedUserProvidedEndpoint: "central.example.com:443",
		},
		{
			name:                         "strips https from hostname",
			input:                        "https://central.example.com:443",
			expectedCentralEndpoint:      "central.example.com:443",
			expectedUserProvidedEndpoint: "central.example.com:443",
		},
		{
			name:                         "strips http prefix",
			input:                        "http://10.0.0.1:443",
			expectedCentralEndpoint:      "10.0.0.1:443",
			expectedUserProvidedEndpoint: "10.0.0.1:443",
		},
		{
			name:                         "strips http from hostname",
			input:                        "http://central.example.com:443",
			expectedCentralEndpoint:      "central.example.com:443",
			expectedUserProvidedEndpoint: "central.example.com:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{}
			d.SetCentralEndpoint(tt.input)

			if d.centralEndpoint != tt.expectedCentralEndpoint {
				t.Errorf("centralEndpoint: got %q, want %q", d.centralEndpoint, tt.expectedCentralEndpoint)
			}
			if d.userProvidedCentralEndpoint != tt.expectedUserProvidedEndpoint {
				t.Errorf("userProvidedCentralEndpoint: got %q, want %q", d.userProvidedCentralEndpoint, tt.expectedUserProvidedEndpoint)
			}
		})
	}
}

func TestGetCentralEndpointForSensor(t *testing.T) {
	tests := []struct {
		name             string
		userProvided     string
		centralNamespace string
		expected         string
	}{
		{
			name:             "falls back to internal endpoint",
			userProvided:     "",
			centralNamespace: "acs-central",
			expected:         "central.acs-central.svc:443",
		},
		{
			name:             "falls back to internal endpoint with custom namespace",
			userProvided:     "",
			centralNamespace: "stackrox",
			expected:         "central.stackrox.svc:443",
		},
		{
			name:             "uses user-provided endpoint",
			userProvided:     "10.0.0.1:443",
			centralNamespace: "acs-central",
			expected:         "10.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Deployer{
				centralNamespace:            tt.centralNamespace,
				userProvidedCentralEndpoint: tt.userProvided,
			}

			result := d.getCentralEndpointForSensor()
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}
