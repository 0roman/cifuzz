package login

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/access_tokens"
	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/login"
	"code-intelligence.com/cifuzz/pkg/log"
)

type loginOpts struct {
	Interactive bool   `mapstructure:"interactive"`
	Server      string `mapstructure:"server"`
}

type loginCmd struct {
	opts      *loginOpts
	apiClient *api.APIClient
}

func New() *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a CI Fuzz Server instance",
		Long: `This command is used to authenticate with a CI Fuzz Server instance.
To learn more, visit https://www.code-intelligence.com.`,
		Example: "$ cifuzz login",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts := &loginOpts{
				Interactive: viper.GetBool("interactive"),
				Server:      viper.GetString("server"),
			}

			// Check if the server option is a valid URL
			err := api.ValidateURL(opts.Server)
			if err != nil {
				// See if prefixing https:// makes it a valid URL
				err = api.ValidateURL("https://" + opts.Server)
				if err != nil {
					log.Error(err, fmt.Sprintf("server %q is not a valid URL", opts.Server))
				}
				opts.Server = "https://" + opts.Server
			}

			apiClient := &api.APIClient{Server: opts.Server}
			cmd := loginCmd{opts: opts, apiClient: apiClient}
			return cmd.run()
		},
	}
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddServerFlag,
	)

	cmdutils.DisableConfigCheck(cmd)

	return cmd
}

func (c *loginCmd) run() error {
	// Obtain the API access token
	var token string
	var err error

	// First, if stdin is *not* a TTY, we try to read it from stdin,
	// in case it was provided via `cifuzz login < token-file`
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// This should never block because stdin is not a TTY.
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return errors.WithStack(err)
		}
		token = strings.TrimSpace(string(b))
		return login.CheckAndStoreToken(c.apiClient, token)
	}

	// Try the access tokens config file
	token = access_tokens.Get(c.opts.Server)
	if token != "" {
		return c.handleExistingToken(token)
	}

	// Try reading it interactively
	if c.opts.Interactive && term.IsTerminal(int(os.Stdin.Fd())) {
		_, err = login.ReadCheckAndStoreTokenInteractively(c.apiClient)
		return err
	}

	err = errors.Errorf(`No API access token provided. Please pass a valid token via stdin or run
in interactive mode. You can generate a token here:
%s/dashboard/settings/account/tokens?create&origin=cli.`+"\n", c.opts.Server)
	return cmdutils.WrapIncorrectUsageError(err)
}

func (c *loginCmd) handleExistingToken(token string) error {
	tokenValid, err := c.apiClient.IsTokenValid(token)
	if err != nil {
		return err
	}
	if !tokenValid {
		err := errors.Errorf(`Failed to authenticate with the configured API access token.
It's possible that the token has been revoked. Please try again after
removing the token from %s.`, access_tokens.GetTokenFilePath())
		log.Warn(err.Error())
		return cmdutils.WrapSilentError(err)
	}
	log.Success("You are already logged in.")
	log.Infof("Your API access token is stored in %s", access_tokens.GetTokenFilePath())
	return nil
}
