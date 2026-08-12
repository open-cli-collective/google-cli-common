package mailcmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// listQuery is the fixed Gmail query behind `mail list`: the inbox, newest
// first — what "list my mail" means to a human. Anything narrower or wider
// is `mail search`'s job.
const listQuery = "in:inbox"

// newListCommand returns `mail list` — sibling symmetry with `calendar list`,
// `contacts list`, and `drive list`, all of which exist while mail previously
// offered only `search`. Without it, `mail list --max 5` failed with cobra's
// unhelpful `unknown flag: --max` (the parent `mail` command swallowed `list`
// as a positional arg and choked on the flag), which reads like a broken
// flag rather than a missing command.
func newListCommand() *cobra.Command {
	var (
		maxResults int64
		idsOnly    bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent inbox messages",
		Long: `List the most recent messages in the inbox (equivalent to
mail search "in:inbox").

Use mail search for anything narrower or wider — archived mail, labels,
senders, date ranges: https://support.google.com/mail/answer/7190

Examples:
  mail list
  mail list --max 25
  mail list --ids | mail mark-read --stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := newGmailClient(cmd.Context())
			if err != nil {
				return fmt.Errorf("creating Gmail client: %w", err)
			}

			if idsOnly {
				ids, err := client.SearchMessageIDs(cmd.Context(), listQuery, maxResults)
				if err != nil {
					return fmt.Errorf("listing messages: %w", err)
				}
				for _, id := range ids {
					fmt.Println(id)
				}
				return nil
			}

			messages, skipped, err := client.SearchMessages(cmd.Context(), listQuery, maxResults)
			if err != nil {
				return fmt.Errorf("listing messages: %w", err)
			}

			if len(messages) == 0 {
				fmt.Println("No messages found.")
				return nil
			}

			for _, msg := range messages {
				printMessageHeader(msg, MessagePrintOptions{
					IncludeThreadID: true,
					IncludeSnippet:  true,
				})
				fmt.Println("---")
			}

			if skipped > 0 {
				fmt.Printf("Note: %d message(s) could not be retrieved.\n", skipped)
			}

			return nil
		},
	}

	cmd.Flags().Int64VarP(&maxResults, "max", "m", 10, "Maximum number of results to return")
	cmd.Flags().BoolVar(&idsOnly, "ids", false, "Output only message IDs (one per line, for piping)")

	return cmd
}
