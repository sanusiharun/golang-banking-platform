package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRoot_DefaultActionRunsServe(t *testing.T) {
	called := 0
	root := NewRoot("auth-svc", func() error {
		called++
		return nil
	})

	// No subcommand → root's default action must run serve, preserving the
	// historical bare-binary behavior.
	root.SetArgs([]string{})
	require.NoError(t, root.Execute())
	assert.Equal(t, 1, called, "bare invocation should run serve exactly once")
}

func TestNewRoot_ServeSubcommandRunsServe(t *testing.T) {
	called := 0
	root := NewRoot("auth-svc", func() error {
		called++
		return nil
	})

	root.SetArgs([]string{"serve"})
	require.NoError(t, root.Execute())
	assert.Equal(t, 1, called, "explicit serve subcommand should run serve")
}

func TestNewRoot_HasServeSubcommand(t *testing.T) {
	root := NewRoot("auth-svc", func() error { return nil })

	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "serve" {
			found = true
		}
		// The auto-generated completion command must stay hidden.
		assert.NotEqual(t, "completion", c.Name(), "completion command should be disabled")
	}
	assert.True(t, found, "serve subcommand should be registered")
}

func TestNewRoot_UnknownSubcommandErrors(t *testing.T) {
	called := 0
	root := NewRoot("auth-svc", func() error {
		called++
		return nil
	})

	root.SetArgs([]string{"bogus"})
	err := root.Execute()
	require.Error(t, err, "unknown subcommand should return an error")
	assert.Zero(t, called, "serve must not run for an unknown subcommand")
}
