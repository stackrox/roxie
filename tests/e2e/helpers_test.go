//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
)

func TestPodLogDumpSelection(t *testing.T) {
	var pods podLogDumpList
	err := json.Unmarshal([]byte(`{
		"items": [
			{
				"metadata": {"name": "running-stable"},
				"status": {"phase": "Running", "containerStatuses": [{"restartCount": 0}]}
			},
			{
				"metadata": {"name": "running-restarted"},
				"status": {"phase": "Running", "containerStatuses": [{"restartCount": 1}, {"restartCount": 2}]}
			},
			{
				"metadata": {"name": "succeeded-init-restarted"},
				"status": {"phase": "Succeeded", "initContainerStatuses": [{"restartCount": 1}]}
			},
			{
				"metadata": {"name": "failed"},
				"status": {"phase": "Failed", "containerStatuses": [{"restartCount": 2}]}
			}
		]
	}`), &pods)
	if err != nil {
		t.Fatalf("unmarshal pod list: %v", err)
	}
	if len(pods.Items) != 4 {
		t.Fatalf("unmarshaled %d pods, want 4", len(pods.Items))
	}

	tests := []struct {
		name             string
		pod              podLogDumpPod
		wantRestartCount int32
		wantDumpLogs     bool
	}{
		{name: "running pod without restarts", pod: pods.Items[0], wantRestartCount: 0, wantDumpLogs: false},
		{name: "running pod with restarts", pod: pods.Items[1], wantRestartCount: 3, wantDumpLogs: true},
		{name: "succeeded pod with init container restarts", pod: pods.Items[2], wantRestartCount: 1, wantDumpLogs: true},
		{name: "failed pod with restarts", pod: pods.Items[3], wantRestartCount: 2, wantDumpLogs: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pod.restartCount(); got != tt.wantRestartCount {
				t.Errorf("restartCount() = %d, want %d", got, tt.wantRestartCount)
			}
			if got := shouldDumpPodLogs(tt.pod.Status.Phase, tt.pod.restartCount()); got != tt.wantDumpLogs {
				t.Errorf("shouldDumpPodLogs() = %t, want %t", got, tt.wantDumpLogs)
			}
		})
	}
}
