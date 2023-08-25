package llvm

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/otiai10/copy"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/mocks"
)

func TestMain(m *testing.M) {
	viper.Set("verbose", true)

	m.Run()
}

func TestIntegration_LLVM(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if runtime.GOOS == "windows" && !config.AllowUnsupportedPlatforms() {
		// TODO: Remove this once https://github.com/microsoft/STL/issues/3568 is fixed.
		t.Skip("This test is broken with Visual Studio 2022")
	}

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	// Include the CMake package by setting the CMAKE_PREFIX_PATH.
	t.Setenv("CMAKE_PREFIX_PATH", filepath.Join(installDir, "share", "cmake"))

	testCases := map[string]struct {
		format string
	}{
		"lcov": {
			format: "lcov",
		},
		"html": {
			format: "html",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cwd, err := os.Getwd()
			require.NoError(t, err)
			testdataDir := filepath.Join(cwd, "testdata")
			testutil.RegisterTestDeps(testdataDir)

			// get path to shared include
			repoRoot, err := builder.FindProjectDir()
			require.NoError(t, err)
			includePath := filepath.Join(repoRoot, "include")

			tmpDir, cleanup := testutil.ChdirToTempDir("llvm-coverage-gen")
			defer cleanup()

			// copy testdata project to tmp directory
			err = copy.Copy(testdataDir, tmpDir)
			require.NoError(t, err)

			// mock finderMock to use include dir from repository
			finderMock := &mocks.RunfilesFinderMock{}
			finderMock.On("CIFuzzIncludePath").Return(includePath, nil)
			finderMock.On("LLVMProfDataPath").Return("llvm-profdata", nil)
			finderMock.On("LLVMCovPath").Return("llvm-cov", nil)

			var bOut bytes.Buffer
			outBuf := io.Writer(&bOut)

			generator := &CoverageGenerator{
				OutputFormat:   tc.format,
				BuildSystem:    "cmake",
				UseSandbox:     false,
				FuzzTest:       "my_fuzz_test",
				ProjectDir:     tmpDir,
				BuildStdout:    outBuf,
				BuildStderr:    os.Stderr,
				Stderr:         os.Stderr,
				runfilesFinder: finderMock,
			}

			err = generator.BuildFuzzTestForCoverage()
			require.NoError(t, err)
			reportPath, err := generator.GenerateCoverageReport()
			require.NoError(t, err)

			if tc.format == "lcov" {
				assert.FileExists(t, reportPath)
				assert.True(t, strings.HasSuffix(reportPath, tc.format))
			} else {
				assert.DirExists(t, reportPath)
				assert.FileExists(t, filepath.Join(reportPath, "index.html"))
			}
		})
	}
}
