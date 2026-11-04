package deployer

import (
	"encoding/json"
	"testing"
)

func TestRouteStatusParse(t *testing.T) {
	raw := `{"spec":{"host":"central-acs-central.apps-crc.testing"},
	         "status":{"ingress":[{"host":"central-acs-central.apps-crc.testing",
	           "conditions":[{"type":"Admitted","status":"True","lastTransitionTime":"x"}]}]}}`
	var r routeStatus
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	if r.Spec.Host != "central-acs-central.apps-crc.testing" {
		t.Fatalf("host: %q", r.Spec.Host)
	}
	host, admitted := routeAdmittedFor(r)
	if !admitted || host != r.Spec.Host {
		t.Fatalf("admitted=%v host=%q", admitted, host)
	}
	// unadmitted / host-mismatch variants
	raw2 := `{"spec":{"host":"h1"},"status":{"ingress":[{"host":"h2","conditions":[{"type":"Admitted","status":"True"}]}]}}`
	json.Unmarshal([]byte(raw2), &r)
	if _, admitted := routeAdmittedFor(r); admitted {
		t.Fatal("host mismatch must not be admitted")
	}
	raw3 := `{"spec":{"host":"h1"},"status":{"ingress":[{"host":"h1","conditions":[{"type":"Admitted","status":"False"}]}]}}`
	json.Unmarshal([]byte(raw3), &r)
	if _, admitted := routeAdmittedFor(r); admitted {
		t.Fatal("Admitted=False must not be admitted")
	}
}
