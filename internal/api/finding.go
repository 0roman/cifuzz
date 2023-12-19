package api

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/finding"
)

type Findings struct {
	Findings []Finding `json:"findings"`
	Links    []Link    `json:"links"`
}

type Finding struct {
	Name                  string       `json:"name"`
	DisplayName           string       `json:"display_name"`
	FuzzTarget            string       `json:"fuzz_target"`
	FuzzingRun            string       `json:"fuzzing_run"`
	CampaignRun           string       `json:"campaign_run"`
	ErrorReport           *ErrorReport `json:"error_report"`
	Timestamp             string       `json:"timestamp"`
	FuzzTargetDisplayName string       `json:"fuzz_target_display_name,omitempty"`

	// new with v3
	JobNid           string       `json:"job_nid"`
	Nid              string       `json:"nid"`
	InputData        string       `json:"input_data"`
	RunNid           string       `json:"run_nid"`
	ErrorID          string       `json:"error_id"`
	Logs             []string     `json:"logs"`
	State            string       `json:"state"`
	CreatedAt        string       `json:"created_at"`
	FirstSeenFinding string       `json:"first_seen_finding"`
	IssueTrackerLink string       `json:"issue_tracker_link"`
	ProjectNid       string       `json:"project_nid"`
	Stacktrace       []Stacktrace `json:"stacktrace"`
}

type Stacktrace struct {
	File     string `json:"file"`
	Function string `json:"function"`
	Line     int64  `json:"line"`
	Column   int64  `json:"column"`
}

type ErrorReport struct {
	Logs      []string `json:"logs"`
	Details   string   `json:"details"`
	Type      string   `json:"type,omitempty"`
	InputData []byte   `json:"input_data,omitempty"`

	DebuggingInfo      *DebuggingInfo        `json:"debugging_info,omitempty"`
	HumanReadableInput string                `json:"human_readable_input,omitempty"`
	MoreDetails        *finding.ErrorDetails `json:"more_details,omitempty"`
	Tag                string                `json:"tag,omitempty"`
	ShortDescription   string                `json:"short_description,omitempty"`
}

type DebuggingInfo struct {
	ExecutablePath string         `json:"executable_path,omitempty"`
	RunArguments   []string       `json:"run_arguments,omitempty"`
	BreakPoints    []*BreakPoint  `json:"break_points,omitempty"`
	Environment    []*Environment `json:"environment,omitempty"`
}

type BreakPoint struct {
	SourceFilePath string           `json:"source_file_path,omitempty"`
	Location       *FindingLocation `json:"location,omitempty"`
	Function       string           `json:"function,omitempty"`
}

type FindingLocation struct {
	Line   uint32 `json:"line,omitempty"`
	Column uint32 `json:"column,omitempty"`
}

type Environment struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type Severity struct {
	Description string  `json:"description,omitempty"`
	Score       float32 `json:"score,omitempty"`
}

// DownloadRemoteFindings downloads all remote findings for a given project from CI Sense.
func (client *APIClient) DownloadRemoteFindings(project string, token string) (*Findings, error) {
	project = ConvertProjectNameForUseWithAPIV1V2(project)

	return APIRequest[Findings](&RequestConfig{
		Client:       client,
		Method:       "GET",
		Token:        token,
		PathSegments: []string{"v1", project, "findings"},
		// setting a timeout of 5 seconds for the request, since we don't want to
		// wait too long, especially when we need to await this request for command
		// completion
		Timeout: 5 * time.Second,
	})

}

// RemoteFindingsForRun uses the v3 API to download all findings for a given
// (container remote-)run.
func (client *APIClient) RemoteFindingsForRun(runNID string, token string) (*Findings, error) {
	return APIRequest[Findings](&RequestConfig{
		Client:       client,
		Method:       "GET",
		Token:        token,
		PathSegments: []string{"v3", "runs", runNID, "findings"},
	})

}

func (client *APIClient) UploadFinding(project string, fuzzTarget string, campaignRunName string, fuzzingRunName string, finding *finding.Finding, token string) error {
	project = ConvertProjectNameForUseWithAPIV1V2(project)

	// loop through the stack trace and create a list of breakpoints
	breakPoints := []*BreakPoint{}
	for _, stackFrame := range finding.StackTrace {
		breakPoints = append(breakPoints, &BreakPoint{
			SourceFilePath: stackFrame.SourceFile,
			Location: &FindingLocation{
				Line:   stackFrame.Line,
				Column: stackFrame.Column,
			},
			Function: stackFrame.Function,
		})
	}

	findings := &Findings{
		Findings: []Finding{
			{
				Name:        project + finding.Name,
				DisplayName: finding.Name,
				FuzzTarget:  fuzzTarget,
				FuzzingRun:  fuzzingRunName,
				CampaignRun: campaignRunName,
				ErrorReport: &ErrorReport{
					Logs:      finding.Logs,
					Details:   finding.Details,
					Type:      string(finding.Type),
					InputData: finding.InputData,
					DebuggingInfo: &DebuggingInfo{
						BreakPoints: breakPoints,
					},
					MoreDetails:      finding.MoreDetails,
					Tag:              finding.Tag,
					ShortDescription: finding.ShortDescriptionColumns()[0],
				},
				Timestamp: time.Now().Format(time.RFC3339),
			},
		},
	}

	body, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = APIRequest[map[string]json.RawMessage](&RequestConfig{
		Client:       client,
		Method:       "POST",
		Body:         body,
		Token:        token,
		PathSegments: []string{"v1", project, "findings"},
	})
	if err != nil {
		return err
	}

	return nil
}
