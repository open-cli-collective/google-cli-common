package mailcmd

import (
	"testing"

	"github.com/open-cli-collective/google-cli-common/testutil"
)

func TestMessagePrintOptions(t *testing.T) {
	t.Parallel()
	t.Run("default options are all false", func(t *testing.T) {
		t.Parallel()
		opts := MessagePrintOptions{}
		testutil.False(t, opts.IncludeThreadID)
		testutil.False(t, opts.IncludeTo)
		testutil.False(t, opts.IncludeSnippet)
		testutil.False(t, opts.IncludeBody)
		testutil.False(t, opts.IncludeQuotedReplyBodies)
	})

	t.Run("options can be set individually", func(t *testing.T) {
		t.Parallel()
		opts := MessagePrintOptions{
			IncludeThreadID: true,
			IncludeBody:     true,
		}
		testutil.True(t, opts.IncludeThreadID)
		testutil.False(t, opts.IncludeTo)
		testutil.False(t, opts.IncludeSnippet)
		testutil.True(t, opts.IncludeBody)
		opts.IncludeQuotedReplyBodies = true
		testutil.True(t, opts.IncludeQuotedReplyBodies)
	})
}
