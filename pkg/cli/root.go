// Package cli provides reusable Cobra command builders shared by all services.
//
// The design goal is progressive enhancement: a service binary invoked with no
// subcommand behaves exactly as it always has (it runs the HTTP server and its
// background workers). Subcommands such as "serve", "migrate", and "gen-keys"
// are additive — available whenever wanted, without changing Docker ENTRYPOINTs
// or compose files.
//
// This package is generic and must never import a service package.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRoot builds a service's root command.
//
// The root's default action (no subcommand given) runs serve, preserving the
// historical bare-binary behavior — `./auth-svc` still boots the server. An
// explicit `serve` subcommand is also registered as a self-documenting alias.
//
// serve is the service's existing run function; its body is unchanged, so
// startup semantics (config load, logger, workers, graceful shutdown) are
// identical to before.
//
// Service-specific commands (migrate, gen-keys, …) are attached by the caller
// via root.AddCommand before Execute.
func NewRoot(name string, serve func() error) *cobra.Command {
	serveRunE := func(_ *cobra.Command, _ []string) error {
		return serve()
	}

	root := &cobra.Command{
		Use:   name,
		Short: name + " — banking platform service",
		// Errors are logged by main() with slog; keep Cobra from double-printing
		// the error and dumping usage on operational (non-usage) failures.
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          serveRunE,
	}

	// Hide the auto-generated `completion` command — services are deployed
	// binaries, not interactive shell tools.
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP server and background workers (default action)",
		Args:  cobra.NoArgs,
		RunE:  serveRunE,
	})

	return root
}
