// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

// Package report writes the machine-readable result and the plain-English summary.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

// Meta is the header of one suite run.
type Meta struct {
	RunID           string `json:"run_id"`
	SourceSHA       string `json:"source_sha"`
	CandidateSHA256 string `json:"candidate_sha256"`
	PreviousSHA256  string `json:"previous_sha256"`
	Tier            string `json:"tier"`
}

// ClaimResult is one registry row after a run.
type ClaimResult struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
}

// StepResult is one runner step.
type StepResult struct {
	ID       string `json:"id"`
	Outcome  string `json:"outcome"`
	Detail   string `json:"detail"`
	Required bool   `json:"required"`
}

// Report is both output files.
type Report struct {
	Meta
	Partition  string        `json:"partition"`
	DiskFaults string        `json:"disk_faults"`
	Claims     []ClaimResult `json:"claims"`
	Steps      []StepResult  `json:"steps"`
	NotProved  []string      `json:"not_proved"`
}

// New starts a report that already says partition and disk faults were not tested.
func New(meta Meta) *Report {
	return &Report{
		Meta:       meta,
		Partition:  labels.PartitionNotTested,
		DiskFaults: labels.DiskFaultsNotTested,
		NotProved: []string{
			"Clock skew is not tested. The nodes share one clock.",
			"Disk faults are not tested.",
			"Real network separation is not tested. The nodes share a loopback interface.",
			"Secure mode is not tested. This slice uses --insecure.",
			"Cross-key serializability is not tested. That checker stays outside this repository.",
			"Statement coverage is not collected in this slice.",
		},
	}
}

// Set records a claim outcome. A later pass does not erase an earlier failure.
func (r *Report) Set(id, outcome, detail string) {
	if r.Outcome(id) == labels.Fail && outcome != labels.Fail {
		return
	}
	for i := range r.Claims {
		if r.Claims[i].ID == id {
			r.Claims[i].Outcome = outcome
			r.Claims[i].Detail = detail
			return
		}
	}
	r.Claims = append(r.Claims, ClaimResult{ID: id, Outcome: outcome, Detail: detail})
}

// Outcome returns the recorded outcome, or an empty string.
func (r *Report) Outcome(id string) string {
	for _, c := range r.Claims {
		if c.ID == id {
			return c.Outcome
		}
	}
	return ""
}

// AddStep appends a runner step.
func (r *Report) AddStep(step StepResult) {
	r.Steps = append(r.Steps, step)
}

// Rejected is true when a claim failed or a required step did not pass.
func (r *Report) Rejected() bool {
	for _, c := range r.Claims {
		if c.Outcome == labels.Fail {
			return true
		}
	}
	for _, s := range r.Steps {
		if s.Required && s.Outcome != labels.Pass {
			return true
		}
	}
	return false
}

// Markdown is the plain-English summary. The first two lines are fixed.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\n", labels.PartitionNotTested, labels.DiskFaultsNotTested)
	if r.Rejected() {
		b.WriteString("result: fail\n\n")
	} else {
		b.WriteString("result: pass\n\n")
	}
	fmt.Fprintf(&b, "Run id: %s\n", blank(r.RunID))
	fmt.Fprintf(&b, "Source sha: %s\n", blank(r.SourceSHA))
	fmt.Fprintf(&b, "Candidate tarball sha256: %s\n", blank(r.CandidateSHA256))
	fmt.Fprintf(&b, "Previous tarball sha256: %s\n", blank(r.PreviousSHA256))
	fmt.Fprintf(&b, "Tier: %s\n\n", blank(r.Tier))
	writeClaims(&b, "Pass", labels.Pass, r.Claims)
	writeClaims(&b, "Fail", labels.Fail, r.Claims)
	writeClaims(&b, "Not tested", labels.NotTested, r.Claims)
	b.WriteString("## Faults not run\n\n")
	fmt.Fprintf(&b, "- %s\n- %s\n\n", r.Partition, r.DiskFaults)
	b.WriteString("## Steps\n\n")
	if len(r.Steps) == 0 {
		b.WriteString("None.\n\n")
	}
	for _, s := range r.Steps {
		fmt.Fprintf(&b, "- %s: %s", s.ID, s.Outcome)
		if s.Detail != "" {
			fmt.Fprintf(&b, " (%s)", oneLine(s.Detail))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Not proved by this slice\n\n")
	for _, line := range r.NotProved {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	return b.String()
}

func writeClaims(b *strings.Builder, title, outcome string, claims []ClaimResult) {
	fmt.Fprintf(b, "## %s\n\n", title)
	n := 0
	for _, c := range claims {
		if c.Outcome != outcome {
			continue
		}
		n++
		fmt.Fprintf(b, "- %s", c.ID)
		if c.Detail != "" {
			fmt.Fprintf(b, ": %s", oneLine(c.Detail))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		b.WriteString("None.\n")
	}
	b.WriteString("\n")
}

func oneLine(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 400 {
		return text[:400] + "..."
	}
	return text
}

func blank(text string) string {
	if text == "" {
		return "(not recorded)"
	}
	return text
}

// Read loads result.json written by Write.
func Read(dir string) (*Report, error) {
	body, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(body, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

// Write creates result.json and summary.md in dir.
func (r *Report) Write(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(dir, "result.json"), body, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(r.Markdown()), 0o644)
}
