package integrationtest

import (
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_WithFileAccesses(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_asan_with_file_accesses")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {

		test := NewLibfuzzerTest(t, buildDir, "trigger_asan_with_file_accesses", disableMinijail)
		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:   finding.ErrorType_CRASH,
			SourceFile:  "trigger_asan_with_file_accesses.c",
			Details:     "heap-buffer-overflow",
			NumFindings: 1,
		})
	})
}
