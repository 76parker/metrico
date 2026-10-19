package noexit_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/76Parker/metrico/cmd/staticlint/noexit"
)

func TestAnalyzer_DirectCallsAndAllowedCases(t *testing.T) {
	results := analysistest.Run(t, analysistest.TestData(), noexit.New(),
		"direct", "renamed", "dot", "other", "shadow", "method")
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			if len(diagnostic.SuggestedFixes) != 0 {
				t.Error("noexit must not suggest an automatic fix")
			}
		}
	}
}
