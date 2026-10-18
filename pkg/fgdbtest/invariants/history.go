// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package invariants

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
)

// Op is one client operation in a register history.
type Op struct {
	Client   int    `json:"client"`
	Kind     string `json:"kind"`
	Value    int    `json:"value"`
	CallNS   int64  `json:"call_ns"`
	ReturnNS int64  `json:"return_ns"`
	Result   string `json:"result"`
}

// ReadHistory loads JSON lines written by the history recorder.
// A trailing partial line is ignored. The recorder may still be appending it.
func ReadHistory(path string) ([]Op, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 && body[len(body)-1] != '\n' {
		cut := bytes.LastIndexByte(body, '\n')
		if cut < 0 {
			return nil, nil
		}
		body = body[:cut+1]
	}
	var ops []Op
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var op Op
		if err := json.Unmarshal([]byte(line), &op); err != nil {
			return nil, fmt.Errorf("history line %d: %w", lineNo, err)
		}
		if err := checkOp(op); err != nil {
			return nil, fmt.Errorf("history line %d: %w", lineNo, err)
		}
		ops = append(ops, op)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ops, nil
}

func checkOp(op Op) error {
	if op.Kind != labels.KindRead && op.Kind != labels.KindWrite {
		return fmt.Errorf("kind %q", op.Kind)
	}
	switch op.Result {
	case labels.ResultOK, labels.ResultFail, labels.ResultUnknown:
	default:
		return fmt.Errorf("result %q", op.Result)
	}
	if op.ReturnNS < op.CallNS {
		return fmt.Errorf("return time is before the call")
	}
	return nil
}

// OKWritesInWindow counts acknowledged writes that both started and returned
// inside [start, end). A write that returns later was not acknowledged in the window.
func OKWritesInWindow(ops []Op, start, end int64) int {
	n := 0
	for _, op := range ops {
		if ackInside(op, start, end) {
			n++
		}
	}
	return n
}

func ackInside(op Op, start, end int64) bool {
	return op.Kind == labels.KindWrite && op.Result == labels.ResultOK &&
		op.CallNS >= start && op.ReturnNS < end
}

// OKWriteValues lists values of acknowledged writes.
func OKWriteValues(ops []Op) []int {
	var values []int
	for _, op := range ops {
		if op.Kind == labels.KindWrite && op.Result == labels.ResultOK {
			values = append(values, op.Value)
		}
	}
	return values
}
