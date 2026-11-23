package application

import "encoding/json"

// pipelinePayload is the subset of a GitLab "Pipeline Hook" body we read. GitLab
// sends many event kinds; only object_kind == "pipeline" carries pipeline state.
type pipelinePayload struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		ID int64 `json:"id"`
	} `json:"project"`
	ObjectAttributes struct {
		ID       int64  `json:"id"`
		Ref      string `json:"ref"`
		SHA      string `json:"sha"`
		Status   string `json:"status"`
		Duration *int   `json:"duration"`
	} `json:"object_attributes"`
	Builds []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Stage  string `json:"stage"`
		Status string `json:"status"`
	} `json:"builds"`
}

// parsePipelinePayload extracts pipeline/job state from a raw webhook body.
//
// isPipeline is false (with a nil error) for any non-pipeline event, which the
// worker treats as nothing-to-reconcile. A malformed body returns an error.
func parsePipelinePayload(raw []byte) (isPipeline bool, gitlabProjectID int64, p GitLabPipeline, jobs []GitLabJob, err error) {
	var body pipelinePayload
	if err := json.Unmarshal(raw, &body); err != nil {
		return false, 0, GitLabPipeline{}, nil, err
	}
	if body.ObjectKind != "pipeline" {
		return false, 0, GitLabPipeline{}, nil, nil
	}

	p = GitLabPipeline{
		ID:       body.ObjectAttributes.ID,
		Ref:      body.ObjectAttributes.Ref,
		SHA:      body.ObjectAttributes.SHA,
		Status:   body.ObjectAttributes.Status,
		Duration: body.ObjectAttributes.Duration,
	}
	jobs = make([]GitLabJob, 0, len(body.Builds))
	for _, b := range body.Builds {
		jobs = append(jobs, GitLabJob{
			ID:     b.ID,
			Name:   b.Name,
			Stage:  b.Stage,
			Status: b.Status,
		})
	}
	return true, body.Project.ID, p, jobs, nil
}
