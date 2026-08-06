package mailcmd

import (
	"testing"

	"github.com/open-cli-collective/google-cli-common/gmail"
	"github.com/open-cli-collective/google-cli-common/testutil"
)

const quotedReplyMarkerForTest = "[quoted reply history elided; use --include-quoted-reply-bodies to show]"

func TestElideQuotedReplyBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		body   string
		isHTML bool
		want   string
		elided bool
	}{
		{
			name:   "Gmail attribution with nested plain quote",
			body:   "Authored reply.\n\nOn [date], [author] wrote:\n>> older line\n>> older line",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Apple attribution with nested plain quote",
			body:   "Authored reply.\n\nOn [date], at [time], [author] wrote:\n\n> older line",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Original Message header block",
			body:   "Authored reply.\n\n-----Original Message-----\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nOlder message",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Reply Message header block",
			body:   "Authored reply.\n\n----- Reply Message -----\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nOlder message",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Outlook header block",
			body:   "Authored reply.\n\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nOlder message",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "CRLF and nested quote depth",
			body:   "Authored reply.\r\n\r\nOn [date], [author] wrote:\r\n\r\n>>> older line\r\n>>>> oldest line",
			want:   "Authored reply.\r\n\r\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "wrapped attribution with two continuation lines",
			body:   "Authored reply.\n\nOn [date], [author]\nwith [context]\nand [more context] wrote:\n> older line",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "wrapped attribution with CRLF",
			body:   "Authored reply.\r\n\r\nOn [date], [author]\r\nwith [context] wrote:\r\n>> older line",
			want:   "Authored reply.\r\n\r\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name: "wrapped attribution followed by inline authored text is preserved",
			body: "Authored reply.\n\nOn [date], [author]\nwith [context] wrote:\n> older line\nInline response",
			want: "Authored reply.\n\nOn [date], [author]\nwith [context] wrote:\n> older line\nInline response",
		},
		{
			name: "quote followed by authored text is preserved",
			body: "Authored reply.\n\nOn [date], [author] wrote:\n> older line\nInline response",
			want: "Authored reply.\n\nOn [date], [author] wrote:\n> older line\nInline response",
		},
		{
			name: "quote authored quote is preserved",
			body: "> older line\nInline response\n> another older line",
			want: "> older line\nInline response\n> another older line",
		},
		{
			name: "forwarded header block is preserved",
			body: "Authored reply.\n\n---------- Forwarded message ----------\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nForwarded message",
			want: "Authored reply.\n\n---------- Forwarded message ----------\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nForwarded message",
		},
		{
			name: "top-level forward is preserved",
			body: "---------- Forwarded message ----------\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nForwarded message",
			want: "---------- Forwarded message ----------\nFrom: [author]\nSent: [date]\nTo: [recipient]\nSubject: [subject]\n\nForwarded message",
		},
		{
			name:   "forward marker nested in terminal quote history is elided",
			body:   "Authored reply.\n\nOn [date], [author] wrote:\n> ---------- Forwarded message ----------\n> From: [author]\n> older message",
			want:   "Authored reply.\n\n" + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name: "unattributed quote block is preserved",
			body: "Authored reply.\n\n> quoted prose\n> more quoted prose",
			want: "Authored reply.\n\n> quoted prose\n> more quoted prose",
		},
		{
			name: "ordinary Markdown quote is preserved",
			body: "# Heading\n\n> quoted prose",
			want: "# Heading\n\n> quoted prose",
		},
		{
			name: "uncertain attribution is preserved",
			body: "Authored reply.\n\nOn [date], [author] wrote:\nnot marked as quoted",
			want: "Authored reply.\n\nOn [date], [author] wrote:\nnot marked as quoted",
		},
		{
			name:   "Gmail HTML quote",
			body:   `<p>Authored reply.</p><div class="gmail_quote"><div class="gmail_attr">On [date], [author] wrote:</div><blockquote>Older message</blockquote></div>`,
			isHTML: true,
			want:   `<p>Authored reply.</p>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Thunderbird HTML quote",
			body:   `<p>Authored reply.</p><blockquote type="cite">Older message</blockquote>`,
			isHTML: true,
			want:   `<p>Authored reply.</p>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Thunderbird attribution sibling and cite block",
			body:   `<p>Authored reply.</p><div class="moz-cite-prefix">On [date], [author] wrote:</div><blockquote type="cite">Older message</blockquote>`,
			isHTML: true,
			want:   `<p>Authored reply.</p>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Outlook HTML quote",
			body:   `<div>Authored reply.</div><div id="divRplyFwdMsg"><div><b>From:</b> [author]</div><div><b>Subject:</b> [subject]</div></div>`,
			isHTML: true,
			want:   `<div>Authored reply.</div>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Outlook border-style header wrapper",
			body:   `<div>Authored reply.</div><div style="border:none;border-top:solid #B5C4DF 1.0pt;padding:3.0pt 0in 0in 0in"><div><b>From:</b> [author]</div><div><b>Sent:</b> [date]</div><div><b>To:</b> [recipient]</div><div><b>Subject:</b> [subject]</div><div>Older message</div></div>`,
			isHTML: true,
			want:   `<div>Authored reply.</div>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "arbitrary styled div is preserved",
			body:   `<div>Authored reply.</div><div style="border-top:solid #B5C4DF 1.0pt"><p>Meaningful styled content</p></div>`,
			isHTML: true,
			want:   `<div>Authored reply.</div><div style="border-top:solid #B5C4DF 1.0pt"><p>Meaningful styled content</p></div>`,
		},
		{
			name:   "Zimbra HTML quote",
			body:   `<div>Authored reply.</div><div id="zmail_extra"><blockquote>Older message</blockquote></div>`,
			isHTML: true,
			want:   `<div>Authored reply.</div>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "Zimbra divider and following history",
			body:   `<div>Authored reply.</div><hr data-marker="__DIVIDER__"><div>Older message</div>`,
			isHTML: true,
			want:   `<div>Authored reply.</div>` + quotedReplyMarkerForTest,
			elided: true,
		},
		{
			name:   "generic HTML blockquote is preserved",
			body:   `<p>Authored reply.</p><blockquote>Quoted prose</blockquote>`,
			isHTML: true,
			want:   `<p>Authored reply.</p><blockquote>Quoted prose</blockquote>`,
		},
		{
			name:   "HTML content after client quote is preserved",
			body:   `<p>Authored reply.</p><div class="gmail_quote">Older message</div><p>Meaningful text after quote</p>`,
			isHTML: true,
			want:   `<p>Authored reply.</p><div class="gmail_quote">Older message</div><p>Meaningful text after quote</p>`,
		},
		{
			name:   "malformed HTML is preserved",
			body:   `<p>Authored reply.<div class="gmail_quote">Older message`,
			isHTML: true,
			want:   `<p>Authored reply.<div class="gmail_quote">Older message`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, elided := elideQuotedReplyBody(tt.body, tt.isHTML)
			testutil.Equal(t, got, tt.want)
			testutil.Equal(t, elided, tt.elided)
		})
	}
}

func TestPrintMessageHeader_ElidesBeforeSanitizing(t *testing.T) {
	msg := &gmail.Message{
		ID:   "synthetic-message",
		Body: "Authored \x1b[31mreply\x1b[0m.\n\nOn [date], [author] wrote:\n> older line",
	}

	output := testutil.CaptureStdout(t, func() {
		printMessageHeader(msg, MessagePrintOptions{IncludeBody: true})
	})

	testutil.Contains(t, output, "Authored reply.")
	testutil.Contains(t, output, quotedReplyMarkerForTest)
	testutil.NotContains(t, output, "older line")
	testutil.NotContains(t, output, "\x1b")
}
