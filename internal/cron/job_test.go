package cron

import (
	"testing"
	"time"
)

func TestJobTypes(t *testing.T) {
	tests := []struct {
		jobType JobType
		valid   bool
	}{
		{JobTypePrompt, true},
		{JobTypeTool, true},
		{JobTypeScript, true},
		{JobType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.jobType), func(t *testing.T) {
			switch tt.jobType {
			case JobTypePrompt, JobTypeTool, JobTypeScript:
				if !tt.valid {
					t.Errorf("expected %s to be invalid", tt.jobType)
				}
			default:
				if tt.valid {
					t.Errorf("expected %s to be valid", tt.jobType)
				}
			}
		})
	}
}

func TestJobCreateValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   JobCreate
		wantErr bool
	}{
		{
			name: "valid prompt job",
			input: JobCreate{
				Name:     "test-job",
				Schedule: "0 * * * *",
				Type:     JobTypePrompt,
				Payload:  `{"prompt": "hello"}`,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			input: JobCreate{
				Schedule: "0 * * * *",
				Type:     JobTypePrompt,
			},
			wantErr: true,
		},
		{
			name: "missing schedule",
			input: JobCreate{
				Name: "test-job",
				Type: JobTypePrompt,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			input: JobCreate{
				Name:     "test-job",
				Schedule: "0 * * * *",
				Type:     JobType("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJobFields(t *testing.T) {
	now := time.Now()
	job := Job{
		Name:      "test-job",
		Schedule:  "0 * * * *",
		Type:      JobTypePrompt,
		Payload:   `{"prompt": "test"}`,
		Enabled:   true,
		LastRun:   &now,
		NextRun:   &now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if job.Name != "test-job" {
		t.Errorf("Name = %s, want test-job", job.Name)
	}
	if job.Type != JobTypePrompt {
		t.Errorf("Type = %s, want prompt", job.Type)
	}
}

func TestHistoryStatus(t *testing.T) {
	tests := []HistoryStatus{
		StatusRunning,
		StatusSuccess,
		StatusFailed,
	}

	for _, status := range tests {
		if status == "" {
			t.Errorf("status should not be empty")
		}
	}
}

func TestHistoryEntry(t *testing.T) {
	now := time.Now()
	entry := HistoryEntry{
		ID:         1,
		JobName:    "test-job",
		StartedAt:  now,
		FinishedAt: &now,
		Status:     StatusSuccess,
		Result:     "done",
		RetryCount: 0,
	}

	if entry.ID != 1 {
		t.Errorf("ID = %d, want 1", entry.ID)
	}
	if entry.Status != StatusSuccess {
		t.Errorf("Status = %s, want success", entry.Status)
	}
}
