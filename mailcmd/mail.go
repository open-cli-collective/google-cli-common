// Package mailcmd implements the mail command and subcommands.
package mailcmd

import (
	"github.com/spf13/cobra"
)

// NewCommand returns the mail parent command with subcommands
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mail",
		Short: "Gmail commands",
		Long: `Access to Gmail messages, threads, attachments, and organizational operations.

This command group provides Gmail functionality:
- list: List recent inbox messages
- search: Search for messages using Gmail query syntax
- read: Read a single message
- thread: Read a full conversation thread
- labels: List all labels
- attachments: List and download attachments
- draft: Compose a draft (never sent automatically)

Organizational operations (non-destructive):
- archive: Remove messages from inbox
- star/unstar: Star or unstar messages
- mark-read/mark-unread: Toggle read status
- label/unlabel: Add or remove user labels
- categorize: Move messages between category tabs

All organizational commands support bulk operations via positional IDs,
--stdin (for piping), or --query (inline search).

Examples:
  mail list
  mail search "is:unread"
  mail read <message-id>
  mail archive --query "from:noreply older_than:30d"
  mail search "is:inbox" --ids | mail star --stdin`,
	}

	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newSearchCommand())
	cmd.AddCommand(newReadCommand())
	cmd.AddCommand(newThreadCommand())
	cmd.AddCommand(newLabelsCommand())
	cmd.AddCommand(newAttachmentsCommand())
	cmd.AddCommand(newArchiveCommand())
	cmd.AddCommand(newStarCommand())
	cmd.AddCommand(newUnstarCommand())
	cmd.AddCommand(newMarkReadCommand())
	cmd.AddCommand(newMarkUnreadCommand())
	cmd.AddCommand(newLabelCommand())
	cmd.AddCommand(newUnlabelCommand())
	cmd.AddCommand(newCategorizeCommand())
	cmd.AddCommand(newDraftCommand())

	return cmd
}
