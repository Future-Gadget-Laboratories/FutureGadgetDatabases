// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package claims

import "strings"

// promiseWords are hints that a new doc sentence may be a claim.
// The check is a warning for reviewers. It does not fail a run.
var promiseWords = []string{
	"always",
	"guarantees",
	"survives",
	"default is",
}

// AddedPromiseLines returns added documentation lines that sound like claims.
// diff is unified diff text. The caller decides whether the registry changed.
func AddedPromiseLines(diff string) []string {
	var found []string
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		low := strings.ToLower(line)
		if containsPromise(low) {
			found = append(found, strings.TrimPrefix(line, "+"))
		}
	}
	return found
}

func containsPromise(low string) bool {
	for _, word := range promiseWords {
		if strings.Contains(low, word) {
			return true
		}
	}
	return false
}
