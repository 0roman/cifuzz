package bundle

import (
	"os"
	"runtime"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

type options struct {
	bundler.Opts `mapstructure:",squash"`
}

func (opts *options) Validate() error {
	if !sliceutil.Contains([]string{
		config.BuildSystemBazel,
		config.BuildSystemCMake,
		config.BuildSystemOther,
		config.BuildSystemMaven,
		config.BuildSystemGradle,
	}, opts.BuildSystem) {
		err := errors.Errorf(`Creating a bundle is currently not supported for %[1]s projects. If you
are interested in using this feature with %[1]s, please file an issue at
https://github.com/CodeIntelligenceTesting/cifuzz/issues`, cases.Title(language.Und).String(opts.BuildSystem))
		log.Print(err.Error())
		return cmdutils.WrapSilentError(err)
	}

	return opts.Opts.Validate()
}

func New() *cobra.Command {
	return newWithOptions(&options{})
}

func newWithOptions(opts *options) *cobra.Command {
	var bindFlags func()
	cmd := &cobra.Command{
		Use:   "bundle [flags] [<fuzz test>]...",
		Short: "Bundles fuzz tests into an archive",
		Long: `This command bundles all runtime artifacts required by the 
given fuzz tests into a self-contained archive (bundle) that can be executed 
by a remote fuzzing server. The usage of this command depends on the build 
system configured for the project.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("CMake") + `
  <fuzz test> is the name of the fuzz test defined in the add_fuzz_test
  command in your CMakeLists.txt.

  Command completion for the <fuzz test> argument is supported when the
  fuzz test was built before or after running 'cifuzz reload'.

  The --build-command flag is ignored.

  If no fuzz tests are specified, all fuzz tests are added to the bundle.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Bazel") + `
  <fuzz test> is the name of the cc_fuzz_test target as defined in your
  BUILD file, either as a relative or absolute Bazel label. 
  
  Command completion for the <fuzz test> argument is supported.

  The '--build-command' flag is ignored.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Other build systems") + `
  <fuzz test> is either the path or basename of the fuzz test executable
  created by the build command. If it's the basename, it will be searched
  for recursively in the current working directory.

  A command which builds the fuzz test executable must be provided via
  the --build-command flag or the build-command setting in cifuzz.yaml.

  The value specified for <fuzz test> is made available to the build
  command in the FUZZ_TEST environment variable. For example:

    echo "build-command: make clean && make \$FUZZ_TEST" >> cifuzz.yaml
    cifuzz run my_fuzz_test

`,
		ValidArgsFunction: completion.ValidFuzzTests,
		Args:              cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}

			// Fail early if the platform is not supported. Creating the
			// bundle actually works on all platforms, but the backend
			// currently only supports running a bundle on Linux, so the
			// user can't do anything useful with a bundle created on
			// other platforms.
			//
			// We set CIFUZZ_BUNDLE_ON_UNSUPPORTED_PLATFORMS in tests to
			// still be able to test that creating the bundle works on
			// all platforms.
			isOSIndependent := opts.BuildSystem == config.BuildSystemMaven ||
				opts.BuildSystem == config.BuildSystemGradle
			if os.Getenv("CIFUZZ_BUNDLE_ON_UNSUPPORTED_PLATFORMS") == "" &&
				runtime.GOOS != "linux" &&
				!isOSIndependent {
				system := cases.Title(language.Und).String(runtime.GOOS)
				if runtime.GOOS == "darwin" {
					system = "macOS"
				}
				err := errors.Errorf(`Creating a bundle is currently only supported on Linux. If you are
interested in using this feature on %s, please file an issue at
https://github.com/CodeIntelligenceTesting/cifuzz/issues`, system)
				log.Print(err.Error())
				return cmdutils.WrapSilentError(err)
			}

			opts.FuzzTests = args
			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts.Stdout = c.OutOrStdout()
			opts.Stderr = c.OutOrStderr()
			return bundler.New(&opts.Opts).Bundle()
		},
	}

	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddBranchFlag,
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddCommitFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddDockerImageFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddTimeoutFlag,
	)
	cmd.Flags().StringVarP(&opts.OutputPath, "output", "o", "", "Output path of the bundle (.tar.gz)")

	return cmd
}
