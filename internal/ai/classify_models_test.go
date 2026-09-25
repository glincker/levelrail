package ai

import "testing"

func TestIsReadOnly_ModelTools(t *testing.T) {
	for _, n := range []string{"list_models", "get_model", "list_gpu_nodes", "get_model_logs"} {
		if !IsReadOnly(n) {
			t.Errorf("%s should be auto-executable read-only", n)
		}
	}
	for _, n := range []string{"deploy_model", "delete_model", "restart_model", "rotate_model_api_key"} {
		if IsReadOnly(n) {
			t.Errorf("%s must require human confirmation in the AI assistant", n)
		}
	}
}
