// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package invariants

import (
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach/pkg/fgdbtest/labels"
	"github.com/cockroachdb/cockroach/pkg/fgdbtest/third_party/porcupine"
)

const emptyRegister = 0

type regIn struct {
	Kind  string
	Value int
}

type regOut struct {
	Result string
	Value  int
}

// CheckHistory runs Porcupine on a single-key register history.
// An empty history, a checker timeout, or an illegal history is an error.
// The check does not skip.
func CheckHistory(ops []Op, timeout time.Duration) error {
	if len(ops) == 0 {
		return fmt.Errorf("porcupine check cannot run: history is empty")
	}
	if !hasOK(ops) {
		return fmt.Errorf("porcupine check cannot run: history has no successful operation")
	}
	history := make([]porcupine.Operation, len(ops))
	for i, op := range ops {
		history[i] = porcupine.Operation{
			ClientId: op.Client,
			Input:    regIn{Kind: op.Kind, Value: op.Value},
			Call:     op.CallNS,
			Output:   regOut{Result: op.Result, Value: op.Value},
			Return:   op.ReturnNS,
		}
	}
	model := registerModel()
	result := porcupine.CheckOperationsTimeout(model, history, timeout)
	if result != porcupine.Ok {
		return fmt.Errorf("porcupine check returned %s", result)
	}
	return nil
}

func hasOK(ops []Op) bool {
	for _, op := range ops {
		if op.Result == labels.ResultOK {
			return true
		}
	}
	return false
}

func registerModel() porcupine.Model {
	spec := porcupine.NondeterministicModel{
		Init: func() []interface{} {
			return []interface{}{emptyRegister}
		},
		Step: func(state, input, output interface{}) []interface{} {
			return stepRegister(state.(int), input.(regIn), output.(regOut))
		},
		Equal: func(a, b interface{}) bool {
			return a.(int) == b.(int)
		},
	}
	return spec.ToModel()
}

func stepRegister(state int, in regIn, out regOut) []interface{} {
	switch in.Kind {
	case labels.KindRead:
		return stepRead(state, out)
	case labels.KindWrite:
		return stepWrite(state, in.Value, out)
	default:
		return nil
	}
}

func stepRead(state int, out regOut) []interface{} {
	if out.Result != labels.ResultOK {
		return []interface{}{state}
	}
	if out.Value != state {
		return nil
	}
	return []interface{}{state}
}

func stepWrite(state, value int, out regOut) []interface{} {
	switch out.Result {
	case labels.ResultOK:
		return []interface{}{value}
	case labels.ResultFail:
		return []interface{}{state}
	case labels.ResultUnknown:
		return unknownWrite(state, value)
	default:
		return nil
	}
}

func unknownWrite(state, value int) []interface{} {
	if state == value {
		return []interface{}{state}
	}
	return []interface{}{state, value}
}
