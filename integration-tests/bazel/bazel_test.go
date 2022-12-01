package bazel

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_Bazel(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if runtime.GOOS == "windows" {
		t.Skip("Using cifuzz with bazel is currently only supported on Unix")
	}

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	testdata := shared.CopyTestdataDir(t, "bazel")
	t.Logf("executing bazel integration test in %s", testdata)
	t.Cleanup(func() { fileutil.Cleanup(testdata) })

	cifuzzRunner := &shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  testdata,
		DefaultFuzzTest: "//src/parser:parser_fuzz_test",
	}

	// Execute the init command
	linesToAdd := cifuzzRunner.Command(t, "init", nil)
	// Append the lines to WORKSPACE
	shared.AppendLines(t, filepath.Join(testdata, "WORKSPACE"), linesToAdd)

	// Execute the create command
	outputPath := filepath.Join("src", "parser", "parser_fuzz_test.cpp")
	linesToAdd = cifuzzRunner.Command(t, "create", &shared.CommandOptions{
		Args: []string{"cpp", "--output", outputPath},
	},
	)

	// Check that the fuzz test was created in the correct directory
	fuzzTestPath := filepath.Join(testdata, outputPath)
	require.FileExists(t, fuzzTestPath)

	// Append the lines to BUILD.bazel
	shared.AppendLines(t, filepath.Join(testdata, "src", "parser", "BUILD.bazel"), linesToAdd)

	t.Run("runEmptyFuzzTest", func(t *testing.T) {
		// Run the (empty) fuzz test
		cifuzzRunner.Run(t, &shared.RunOptions{
			ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`^paths: \d+`)},
			TerminateAfterExpectedOutput: true,
		})
	})

	// Make the fuzz test call a function
	shared.ModifyFuzzTestToCallFunction(t, fuzzTestPath)

	// Add dependency on parser lib to BUILD.bazel
	cmd := exec.Command("buildozer", "add deps :parser", "//src/parser:parser_fuzz_test")
	cmd.Dir = testdata
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err)

	t.Run("noCIFuzz", func(t *testing.T) {
		testNoCIFuzz(t, cifuzzRunner)
	})

	t.Run("runWithAdditionalArgs", func(t *testing.T) {
		testRunWithAdditionalArgs(t, cifuzzRunner)
	})

	t.Run("runAndLCOV", func(t *testing.T) {
		testRun(t, cifuzzRunner)
		// Requires the generated corpus to be populated by run.
		testLCOVCoverage(t, cifuzzRunner)
	})

	t.Run("bundle", func(t *testing.T) {
		testBundle(t, cifuzzRunner)
	})

	t.Run("remoteRun", func(t *testing.T) {
		testRemoteRun(t, cifuzzRunner)
	})

	t.Run("coverage", func(t *testing.T) {
		testCoverage(t, cifuzzRunner)
	})
}

func testRunWithAdditionalArgs(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// Run bazel and expect it to fail because we passed it a non-existent flag
	cifuzzRunner.Run(t, &shared.RunOptions{
		Args: []string{"--", "--non-existent-flag"},
		ExpectedOutputs: []*regexp.Regexp{
			regexp.MustCompile(`Unrecognized option: --non-existent-flag`),
		},
		ExpectError: true,
	})
}

func testRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	t.Parallel()
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir

	// Run the fuzz test and check that it finds the use-after-free
	expectedOutputs := []*regexp.Regexp{
		// Check that the use-after-free is found
		regexp.MustCompile(`^==\d*==ERROR: AddressSanitizer: heap-use-after-free`),
	}

	// Check that Minijail is used (if running on Linux, because Minijail
	// is only supported on Linux)
	if runtime.GOOS == "linux" {
		expectedOutputs = append(expectedOutputs, regexp.MustCompile(`bin/minijail0`))
	}

	cifuzzRunner.Run(t, &shared.RunOptions{ExpectedOutputs: expectedOutputs})

	// Check that the findings command lists the findings
	findings := shared.GetFindings(t, cifuzz, testdata)
	require.Len(t, findings, 2)

	var asanFinding *finding.Finding
	var ubsanFinding *finding.Finding
	for _, f := range findings {
		if strings.HasPrefix(f.Details, "heap-use-after-free") {
			asanFinding = f
		} else if strings.HasPrefix(f.Details, "undefined behavior") {
			ubsanFinding = f
		} else {
			t.Fatalf("unexpected finding: %q", f.Details)
		}
	}

	// Verify that there is an ASan finding and that it has the correct details.
	require.NotNil(t, asanFinding)
	// TODO: This check currently fails on macOS because there
	// llvm-symbolizer doesn't read debug info from object files.
	// See https://github.com/google/sanitizers/issues/207#issuecomment-136495556
	if runtime.GOOS != "darwin" {
		expectedStackTrace := []*stacktrace.StackFrame{
			{
				SourceFile:  "src/parser/parser.cpp",
				Line:        19,
				Column:      14,
				FrameNumber: 0,
				Function:    "parse",
			},
			{
				SourceFile:  "src/parser/parser_fuzz_test.cpp",
				Line:        30,
				Column:      3,
				FrameNumber: 1,
				Function:    "LLVMFuzzerTestOneInputNoReturn",
			},
		}
		if runtime.GOOS == "windows" {
			// On Windows, the column is not printed
			for i := range expectedStackTrace {
				expectedStackTrace[i].Column = 0
			}
		}
		require.Equal(t, expectedStackTrace, asanFinding.StackTrace)
	}

	// Verify that there is a UBSan finding and that it has the correct details.
	require.NotNil(t, ubsanFinding)
	require.NotEmpty(t, ubsanFinding.InputFile)
	// Verify that UBSan findings come with inputs.
	// TODO: Use when we also dump the input of UBSan findings with bazel
	//require.NotEmpty(t, ubsanFinding.InputFile)
	if runtime.GOOS != "darwin" {
		expectedStackTrace := []*stacktrace.StackFrame{
			{
				SourceFile:  "src/parser/parser.cpp",
				Line:        23,
				Column:      9,
				FrameNumber: 0,
				Function:    "parse",
			},
			{
				SourceFile:  "src/parser/parser_fuzz_test.cpp",
				Line:        30,
				Column:      3,
				FrameNumber: 1,
				Function:    "LLVMFuzzerTestOneInputNoReturn",
			},
		}
		require.Equal(t, expectedStackTrace, ubsanFinding.StackTrace)
	}

	// Check that ASAN_OPTIONS can be set
	env, err := envutil.Setenv(os.Environ(), "ASAN_OPTIONS", "print_stats=1:atexit=1")
	require.NoError(t, err)
	cifuzzRunner.Run(t, &shared.RunOptions{
		Env:                          env,
		ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`Stats:`)},
		TerminateAfterExpectedOutput: false,
	})
}

func testBundle(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	if runtime.GOOS == "darwin" {
		t.Skip("Bundle is currently not supported on macOS")
	}
	t.Parallel()
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	// Run cifuzz bundle and verify the contents of the archive.
	shared.TestBundle(t, testdata, cifuzz, "//src/parser:parser_fuzz_test", "//src/bundle:ubsan_function_ptr_fuzz_test")
}

func testRemoteRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// The remote-run command is currently only supported on Linux
	if runtime.GOOS != "linux" {
		t.Skip()
	}
	t.Parallel()

	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	shared.TestRemoteRun(t, testdata, cifuzz, "//src/parser:parser_fuzz_test")
}

func testCoverage(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// TODO: fix coverage on macOS CI
	if runtime.GOOS == "darwin" {
		t.Skip("Coverage is currently not working on our macOS CI")
	}
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir

	cmd := executil.Command(cifuzz, "coverage",
		"--verbose",
		"--output", "coverage-report",
		"//src/parser:parser_fuzz_test")
	cmd.Dir = testdata
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(testdata, "coverage-report", "parser", "index.html")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for the
	// parser.cpp source file, but not for our headers.
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	report := string(reportBytes)
	require.Contains(t, report, "parser.cpp")
	require.NotContains(t, report, "include/cifuzz")
}

func testNoCIFuzz(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cmd := exec.Command("bazel", "test", "//...")
	cmd.Dir = cifuzzRunner.DefaultWorkDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run())
}

func testLCOVCoverage(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// TODO: fix coverage on macOS CI
	if runtime.GOOS == "darwin" {
		t.Skip("Coverage is currently not working on our macOS CI")
	}
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir

	// Temporarily rename the crashing inputs to verify that the following
	// coverage command operates on the generated corpus only.
	fuzzTestInputs := filepath.Join(testdata, "src", "parser", "parser_fuzz_test_inputs")
	backupFuzzTestInputs := filepath.Join(testdata, "src", "parser", "parser_fuzz_test_inputs.backup")
	err := os.Rename(fuzzTestInputs, backupFuzzTestInputs)
	require.NoError(t, err)
	t.Cleanup(func() { os.Rename(backupFuzzTestInputs, fuzzTestInputs) }) //nolint:errcheck

	cmd := executil.Command(cifuzz, "coverage",
		"--verbose",
		"--format=lcov",
		"--output", "report.lcov",
		"//src/parser:parser_fuzz_test")
	cmd.Dir = testdata
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(testdata, "report.lcov")
	require.FileExists(t, reportPath)

	// Read the report and extract all uncovered lines in the fuzz test source file.
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	lcov := bufio.NewScanner(bytes.NewBuffer(reportBytes))
	isParserSource := false
	var uncoveredLines []uint
	for lcov.Scan() {
		line := lcov.Text()

		if strings.HasPrefix(line, "SF:") {
			if strings.HasSuffix(line, "/parser/parser.cpp") {
				isParserSource = true
			} else {
				isParserSource = false
			}
		}

		if !isParserSource || !strings.HasPrefix(line, "DA:") {
			continue
		}
		split := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
		require.Len(t, split, 2)
		if split[1] == "0" {
			lineNo, err := strconv.Atoi(split[0])
			require.NoError(t, err)
			uncoveredLines = append(uncoveredLines, uint(lineNo))
		}
	}

	assert.Subset(t, []uint{
		// The generated corpus never contains the empty input.
		11, 12,
		// The generated corpus does not contain the crashing inputs.
		15, 16, 19},
		uncoveredLines)
}
