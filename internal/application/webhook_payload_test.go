package application

import "testing"

func TestParsePipelinePayload_MergeRequestIID(t *testing.T) {
	const withMR = `{
		"object_kind":"pipeline",
		"project":{"id":1001},
		"object_attributes":{"id":55,"ref":"feature","sha":"abc","status":"failed"},
		"merge_request":{"iid":42},
		"builds":[{"id":9,"name":"test","stage":"test","status":"failed"}]
	}`

	isPipeline, projectID, mrIID, _, jobs, err := parsePipelinePayload([]byte(withMR))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPipeline {
		t.Fatal("isPipeline = false, want true")
	}
	if projectID != 1001 {
		t.Errorf("projectID = %d, want 1001", projectID)
	}
	if mrIID != 42 {
		t.Errorf("mergeRequestIID = %d, want 42", mrIID)
	}
	if len(jobs) != 1 {
		t.Errorf("jobs = %d, want 1", len(jobs))
	}
}

func TestParsePipelinePayload_NoMergeRequest(t *testing.T) {
	const noMR = `{
		"object_kind":"pipeline",
		"project":{"id":1001},
		"object_attributes":{"id":55,"ref":"main","sha":"abc","status":"failed"},
		"builds":[]
	}`

	isPipeline, _, mrIID, _, _, err := parsePipelinePayload([]byte(noMR))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPipeline {
		t.Fatal("isPipeline = false, want true")
	}
	if mrIID != 0 {
		t.Errorf("mergeRequestIID = %d, want 0 when no merge_request present", mrIID)
	}
}

func TestParsePipelinePayload_NonPipelineSkipped(t *testing.T) {
	isPipeline, _, mrIID, _, _, err := parsePipelinePayload([]byte(`{"object_kind":"push","project":{"id":1001}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isPipeline {
		t.Error("isPipeline = true, want false for a push event")
	}
	if mrIID != 0 {
		t.Errorf("mergeRequestIID = %d, want 0", mrIID)
	}
}
