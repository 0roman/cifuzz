package execute

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	runCmd "code-intelligence.com/cifuzz/internal/cmd/run"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type executeOpts struct {
	SingleFuzzTest bool `mapstructure:"single-fuzz-test"`

	name string
}

type executeCmd struct {
	*cobra.Command
	opts *executeOpts
}

func New() *cobra.Command {
	opts := &executeOpts{}
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute a fuzz test bundle locally",
		Long: `This command executes a cifuzz fuzz test bundle locally.
It can be used as an experimental alternative to cifuzz_runner.
It is currently only intended for use with the 'cifuzz container' subcommand.

`,
		Example: "cifuzz execute [fuzz test]",
		Args:    cobra.MaximumNArgs(1),
		PreRun: func(cmd *cobra.Command, args []string) {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			cmdutils.ViperMustBindPFlag("single-fuzz-test", cmd.Flags().Lookup("single-fuzz-test"))
			opts.SingleFuzzTest = viper.GetBool("single-fuzz-test")
		},
		RunE: func(c *cobra.Command, args []string) error {
			// If there are no arguments provided, provide a helpful message and list all available fuzzers.
			if len(args) == 0 && !opts.SingleFuzzTest {
				metadata, err := getMetadata()
				if err != nil {
					return err
				}

				_ = pterm.DefaultBigText.WithLetters(
					putils.LettersFromStringWithStyle("Fuzz", pterm.FgCyan.ToStyle()),
					putils.LettersFromString(" "),
					putils.LettersFromStringWithStyle("Container", pterm.FgLightMagenta.ToStyle())).
					Render()

				fmt.Println("")
				fmt.Printf("This container is based on: %s\n", metadata.RunEnvironment.Docker)
				fmt.Println("")

				fmt.Printf("Available fuzzers:\n")
				for _, fuzzer := range metadata.Fuzzers {
					fuzzerName := fuzzer.Name
					if fuzzerName == "" {
						fuzzerName = fuzzer.Name
					}
					fmt.Printf("  %s\n", fuzzerName)
					fmt.Printf("    using: %s\n", fuzzer.Engine)
					fmt.Printf("    run fuzz test with: cifuzz execute %s\n", fuzzerName)
					fmt.Println("")
				}
				return nil
			}

			if opts.SingleFuzzTest && len(args) > 0 {
				msg := "The <fuzz test> argument cannot be used with the --single-fuzz-test flag."
				return cmdutils.WrapIncorrectUsageError(errors.New(msg))
			}

			if !opts.SingleFuzzTest {
				opts.name = args[0]
			}

			cmd := executeCmd{Command: c, opts: opts}
			return cmd.run()
		},
	}

	cmdutils.DisableConfigCheck(cmd)

	cmd.Flags().Bool("single-fuzz-test", false, "Run the only fuzz test in the bundle (without specifying the fuzz test name).")

	return cmd
}

func (c *executeCmd) run() error {
	metadata, err := getMetadata()
	if err != nil {
		return err
	}

	fuzzer, err := findFuzzer(c.opts.name, metadata)
	if err != nil {
		return err
	}

	runner, err := buildRunner(fuzzer)
	if err != nil {
		return err
	}
	return runCmd.ExecuteRunner(runner)
}

// getMetadata returns the bundle metadata from the bundle.yaml file.
func getMetadata() (*archive.Metadata, error) {
	exists, err := fileutil.Exists(archive.MetadataFileName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !exists {
		return nil, errors.Errorf("bundle metadata file '%s' does not exist. Execute command should be run in a folder with an unpacked cifuzz bundle.", archive.MetadataFileName)
	}

	metadataFile, err := os.ReadFile(archive.MetadataFileName)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	metadata := &archive.Metadata{}
	err = metadata.FromYaml(metadataFile)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// getFuzzerName returns the fuzzer name. Some Fuzzer define Name (jazzer) and some define Target (libfuzzer).
func getFuzzerName(fuzzer *archive.Fuzzer) string {
	if fuzzer.Name != "" {
		return fuzzer.Name
	}
	return fuzzer.Target
}

// findFuzzer returns the fuzzer with the given name in Fuzzers list in Bundle Metadata.
func findFuzzer(nameToFind string, bundleMetadata *archive.Metadata) (*archive.Fuzzer, error) {
	// libFuzzer fuzz tests contain two entries in the metadata file, one
	// for fuzzing and one for coverage. We want the fuzzing entries, which
	// are listed first.
	fuzzers := make(map[string]*archive.Fuzzer)
	for _, fuzzer := range bundleMetadata.Fuzzers {
		name := getFuzzerName(fuzzer)
		if _, ok := fuzzers[name]; !ok {
			fuzzers[name] = fuzzer
		}
	}

	if nameToFind == "" {
		// Check if there is only one fuzzer in the bundle.
		if len(fuzzers) == 1 {
			// Return the only fuzzer in the bundle.
			for _, fuzzer := range fuzzers {
				return fuzzer, nil
			}
		}
		return nil, errors.Errorf("no fuzzer name provided and more than one fuzzer found in a bundle metadata file")
	}

	if fuzzer, ok := fuzzers[nameToFind]; ok {
		// TODO: is there a more validation we want to perform? If so, should it be part of the metadata parsing?
		// TODO: is multiple matches a valid scenario?
		return fuzzer, nil
	}

	return nil, errors.Errorf("fuzzer '%s' not found in a bundle metadata file", nameToFind)
}

func buildRunner(fuzzer *archive.Fuzzer) (runCmd.Runner, error) {
	// TODO: create or get real directory for seed corpus
	corpusDirName := "corpus"
	seedDirName := "seed"
	err := os.MkdirAll(seedDirName, 0o755)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = os.MkdirAll(corpusDirName, 0o755)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	reportHandler, err := reporthandler.NewReportHandler(
		getFuzzerName(fuzzer),
		&reporthandler.ReportHandlerOptions{
			ProjectDir:           fuzzer.ProjectDir,
			PrintJSON:            false,
			ManagedSeedCorpusDir: seedDirName,
		})
	if err != nil {
		return nil, err
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		FuzzTarget:         fuzzer.Path,
		ProjectDir:         fuzzer.ProjectDir,
		UseMinijail:        false,
		LibraryDirs:        fuzzer.LibraryPaths,
		Verbose:            true, // Should this respect -v flag?
		ReportHandler:      reportHandler,
		GeneratedCorpusDir: corpusDirName,
		EnvVars:            []string{"NO_CIFUZZ=1"},
	}

	var runner runCmd.Runner

	switch fuzzer.Engine {
	case "JAVA_LIBFUZZER":

		name := fuzzer.Name
		method := ""
		if strings.Contains(fuzzer.Name, "::") {
			split := strings.Split(fuzzer.Name, "::")
			name = split[0]
			method = split[1]
		}
		runnerOpts := &jazzer.RunnerOptions{
			TargetClass:      name,
			TargetMethod:     method,
			ClassPaths:       fuzzer.RuntimePaths,
			LibfuzzerOptions: runnerOpts,
		}
		runner = jazzer.NewRunner(runnerOpts)
	default:
		runner = libfuzzer.NewRunner(runnerOpts)
	}

	return runner, nil
}
