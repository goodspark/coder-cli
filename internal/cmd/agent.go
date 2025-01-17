package cmd

import (
	"net/url"
	"os"
	"os/signal"
	"syscall"

	// We use slog here since agent runs in the background and we can benefit
	// from structured logging.
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/coder-cli/wsnet"
)

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "agent",
		Short:  "Run the workspace agent",
		Long:   "Connect to Coder and start running a p2p agent",
		Hidden: true,
	}

	cmd.AddCommand(
		startCmd(),
	)
	return cmd
}

func startCmd() *cobra.Command {
	var (
		token    string
		coderURL string
	)
	cmd := &cobra.Command{
		Use:   "start --coder-url=[coder_url] --token=[token]",
		Short: "starts the coder agent",
		Long:  "starts the coder agent",
		Example: `# start the agent and use CODER_URL and CODER_AGENT_TOKEN env vars

coder agent start

# start the agent and connect with a specified url and agent token

coder agent start --coder-url https://my-coder.com --token xxxx-xxxx
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx = cmd.Context()
				log = slog.Make(sloghuman.Sink(os.Stderr)).Leveled(slog.LevelDebug)
			)
			if coderURL == "" {
				var ok bool
				coderURL, ok = os.LookupEnv("CODER_URL")
				if !ok {
					client, err := newClient(ctx, true)
					if err != nil {
						return xerrors.New("must login, pass --coder-url flag, or set the CODER_URL env variable")
					}
					burl := client.BaseURL()
					coderURL = burl.String()
				}
			}

			u, err := url.Parse(coderURL)
			if err != nil {
				return xerrors.Errorf("parse url: %w", err)
			}

			if token == "" {
				var ok bool
				token, ok = os.LookupEnv("CODER_AGENT_TOKEN")
				if !ok {
					return xerrors.New("must pass --token or set the CODER_AGENT_TOKEN env variable")
				}
			}

			log.Info(ctx, "starting wsnet listener", slog.F("coder_access_url", u.String()))
			listener, err := wsnet.Listen(ctx, log, wsnet.ListenEndpoint(u, token), token)
			if err != nil {
				return xerrors.Errorf("listen: %w", err)
			}
			defer func() {
				log.Info(ctx, "closing wsnet listener")
				err := listener.Close()
				if err != nil {
					log.Error(ctx, "close listener", slog.Error(err))
				}
			}()

			// Block until user sends SIGINT or SIGTERM
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			<-sigs

			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "coder agent token")
	cmd.Flags().StringVar(&coderURL, "coder-url", "", "coder access url")

	return cmd
}
