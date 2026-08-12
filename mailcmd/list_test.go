package mailcmd

import (
	"context"
	"testing"

	gmailapi "github.com/open-cli-collective/google-cli-common/gmail"
	"github.com/open-cli-collective/google-cli-common/testutil"
)

func TestListCommand(t *testing.T) {
	cmd := newListCommand()

	t.Run("has correct use", func(t *testing.T) {
		testutil.Equal(t, cmd.Use, "list")
	})

	t.Run("takes no arguments", func(t *testing.T) {
		err := cmd.Args(cmd, []string{})
		testutil.NoError(t, err)

		err = cmd.Args(cmd, []string{"unexpected"})
		testutil.Error(t, err)
	})

	// Flag parity with mail search: `mail search --max N` working while
	// `mail list --max N` fails is exactly the divergence this command
	// exists to close.
	t.Run("has max flag matching search", func(t *testing.T) {
		flag := cmd.Flags().Lookup("max")
		testutil.NotNil(t, flag)
		testutil.Equal(t, flag.Shorthand, "m")
		testutil.Equal(t, flag.DefValue, "10")
	})

	t.Run("has ids flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("ids")
		testutil.NotNil(t, flag)
		testutil.Equal(t, flag.DefValue, "false")
	})
}

func TestListCommand_Success(t *testing.T) {
	mock := &MockGmailClient{
		SearchMessagesFunc: func(_ context.Context, query string, maxResults int64) ([]*gmailapi.Message, int, error) {
			testutil.Equal(t, query, listQuery)
			testutil.Equal(t, maxResults, int64(10))
			return testutil.SampleMessages(2), 0, nil
		},
	}

	cmd := newListCommand()
	cmd.SetArgs([]string{})

	withMockClient(mock, func() {
		output := testutil.CaptureStdout(t, func() {
			err := cmd.Execute()
			testutil.NoError(t, err)
		})

		testutil.Contains(t, output, "ID: msg_a")
		testutil.Contains(t, output, "ID: msg_b")
	})
}

func TestListCommand_IDsOnly(t *testing.T) {
	mock := &MockGmailClient{
		SearchMessageIDsFunc: func(_ context.Context, query string, maxResults int64) ([]string, error) {
			testutil.Equal(t, query, listQuery)
			testutil.Equal(t, maxResults, int64(25))
			return []string{"msg_a", "msg_b"}, nil
		},
	}

	cmd := newListCommand()
	cmd.SetArgs([]string{"--ids", "--max", "25"})

	withMockClient(mock, func() {
		output := testutil.CaptureStdout(t, func() {
			err := cmd.Execute()
			testutil.NoError(t, err)
		})

		testutil.Equal(t, output, "msg_a\nmsg_b\n")
	})
}

func TestListCommand_NoResults(t *testing.T) {
	mock := &MockGmailClient{
		SearchMessagesFunc: func(_ context.Context, _ string, _ int64) ([]*gmailapi.Message, int, error) {
			return []*gmailapi.Message{}, 0, nil
		},
	}

	cmd := newListCommand()
	cmd.SetArgs([]string{})

	withMockClient(mock, func() {
		output := testutil.CaptureStdout(t, func() {
			err := cmd.Execute()
			testutil.NoError(t, err)
		})

		testutil.Contains(t, output, "No messages found.")
	})
}
