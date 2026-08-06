package mailcmd

import (
	"bytes"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

const quotedReplyElisionMarker = "[quoted reply history elided; use --include-quoted-reply-bodies to show]"

var (
	replyAttributionLine  = regexp.MustCompile(`(?i)^on\b.*\bwrote:\s*$`)
	replyAttributionStart = regexp.MustCompile(`(?i)^on\b`)
	replyAttributionEnd   = regexp.MustCompile(`(?i)\bwrote:\s*$`)
)

type replyTextLine struct {
	start int
	text  string
}

// elideQuotedReplyBody removes only a confidently identified terminal quote.
// It returns the original body and false when the shape is ambiguous.
func elideQuotedReplyBody(body string, bodyIsHTML bool) (string, bool) {
	if bodyIsHTML {
		return elideQuotedReplyHTML(body)
	}
	return elideQuotedReplyPlain(body)
}

func elideQuotedReplyPlain(body string) (string, bool) {
	lines := splitReplyTextLines(body)
	if len(lines) == 0 || hasTopLevelForwardedMessage(lines) {
		return body, false
	}

	for i, line := range lines {
		if attributionEnd, ok := replyAttributionEndLine(lines, i); ok && hasQuotedSuffix(lines, attributionEnd) {
			return body[:line.start] + quotedReplyElisionMarker, true
		}
		trimmed := strings.TrimSpace(line.text)
		if isReplyHistoryMarker(trimmed) && hasReplyHeaderBlock(lines, i+1) {
			return body[:line.start] + quotedReplyElisionMarker, true
		}
		if isOutlookHeaderStart(lines, i) && hasReplyHeaderBlock(lines, i) {
			return body[:line.start] + quotedReplyElisionMarker, true
		}
	}

	return body, false
}

func splitReplyTextLines(body string) []replyTextLine {
	if body == "" {
		return nil
	}

	var lines []replyTextLine
	for start := 0; start < len(body); {
		end := strings.IndexByte(body[start:], '\n')
		if end < 0 {
			end = len(body)
		} else {
			end += start + 1
		}
		text := body[start:end]
		text = strings.TrimSuffix(text, "\n")
		text = strings.TrimSuffix(text, "\r")
		lines = append(lines, replyTextLine{start: start, text: text})
		start = end
	}
	return lines
}

func hasTopLevelForwardedMessage(lines []replyTextLine) bool {
	for _, line := range lines {
		if isPlainQuoteLine(line.text) {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(line.text))
		if strings.Contains(text, "forwarded message") &&
			(strings.Contains(text, "-") || strings.HasPrefix(text, "begin ")) {
			return true
		}
	}
	return false
}

func replyAttributionEndLine(lines []replyTextLine, start int) (int, bool) {
	line := strings.TrimSpace(lines[start].text)
	if replyAttributionLine.MatchString(line) {
		return start, true
	}
	if !replyAttributionStart.MatchString(line) {
		return 0, false
	}

	last := start + 2
	if last >= len(lines) {
		last = len(lines) - 1
	}
	for end := start + 1; end <= last; end++ {
		continuation := strings.TrimSpace(lines[end].text)
		if continuation == "" || isPlainQuoteLine(continuation) {
			return 0, false
		}
		if replyAttributionEnd.MatchString(continuation) {
			return end, true
		}
	}
	return 0, false
}

func hasQuotedSuffix(lines []replyTextLine, attribution int) bool {
	hasContent := false
	for _, line := range lines[attribution+1:] {
		text := strings.TrimSpace(line.text)
		if text == "" {
			continue
		}
		if !isPlainQuoteLine(text) {
			return false
		}
		if plainQuoteContent(text) != "" {
			hasContent = true
		}
	}
	return hasContent
}

func isPlainQuoteLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), ">")
}

func plainQuoteContent(line string) string {
	line = strings.TrimLeft(line, " \t")
	for strings.HasPrefix(line, ">") {
		line = strings.TrimLeft(line[1:], " \t")
	}
	return strings.TrimSpace(line)
}

func isReplyHistoryMarker(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "-") || !strings.HasSuffix(line, "-") {
		return false
	}
	lower := strings.ToLower(line)
	return strings.Contains(lower, "original message") || strings.Contains(lower, "reply message")
}

func isOutlookHeaderStart(lines []replyTextLine, index int) bool {
	if replyHeaderName(lines[index].text) != "from" {
		return false
	}
	return index == 0 || strings.TrimSpace(lines[index-1].text) == ""
}

func hasReplyHeaderBlock(lines []replyTextLine, start int) bool {
	for start < len(lines) && strings.TrimSpace(lines[start].text) == "" {
		start++
	}

	seen := make(map[string]bool)
	for start < len(lines) {
		name := replyHeaderName(lines[start].text)
		if name == "" {
			break
		}
		seen[name] = true
		start++
	}

	if !seen["from"] || !seen["to"] || !seen["subject"] || (!seen["sent"] && !seen["date"]) {
		return false
	}
	for _, line := range lines[start:] {
		if strings.TrimSpace(line.text) != "" {
			return true
		}
	}
	return false
}

func replyHeaderName(line string) string {
	colon := strings.IndexByte(line, ':')
	if colon < 1 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(line[:colon])) {
	case "from", "sent", "date", "to", "cc", "subject":
		return strings.ToLower(strings.TrimSpace(line[:colon]))
	default:
		return ""
	}
}

func elideQuotedReplyHTML(body string) (string, bool) {
	doc, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return body, false
	}
	bodyNode := findHTMLBody(doc)
	if bodyNode == nil {
		return body, false
	}
	candidate := terminalHTMLQuote(bodyNode)
	if candidate == nil || !htmlQuoteHasExplicitClose(body, candidate) {
		return body, false
	}

	parent := candidate.Parent
	if parent == nil {
		return body, false
	}
	first := candidate
	if previous := previousMeaningfulHTMLSibling(candidate); isHTMLQuoteAttribution(previous) {
		first = previous
	}
	next := candidate.NextSibling
	if isZimbraDivider(candidate) {
		for node := candidate; node != nil; {
			nextNode := node.NextSibling
			parent.RemoveChild(node)
			node = nextNode
		}
		next = nil
	} else {
		for node := first; ; {
			nextNode := node.NextSibling
			parent.RemoveChild(node)
			if node == candidate {
				break
			}
			node = nextNode
		}
	}
	marker := &xhtml.Node{Type: xhtml.TextNode, Data: quotedReplyElisionMarker}
	parent.InsertBefore(marker, next)

	var out bytes.Buffer
	if isHTMLDocument(body) {
		if err := xhtml.Render(&out, doc); err != nil {
			return body, false
		}
	} else {
		for node := bodyNode.FirstChild; node != nil; node = node.NextSibling {
			if err := xhtml.Render(&out, node); err != nil {
				return body, false
			}
		}
	}
	return out.String(), true
}

func findHTMLBody(node *xhtml.Node) *xhtml.Node {
	if node.Type == xhtml.ElementNode && node.Data == "body" {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if body := findHTMLBody(child); body != nil {
			return body
		}
	}
	return nil
}

func terminalHTMLQuote(parent *xhtml.Node) *xhtml.Node {
	if divider := terminalHTMLDivider(parent); divider != nil {
		return divider
	}
	last := lastMeaningfulHTMLChild(parent)
	if last == nil {
		return nil
	}
	if isHTMLQuoteCandidate(last) && htmlHasMeaningfulContent(last) {
		return last
	}
	if last.Type == xhtml.ElementNode {
		return terminalHTMLQuote(last)
	}
	return nil
}

func terminalHTMLDivider(parent *xhtml.Node) *xhtml.Node {
	var divider *xhtml.Node
	for child := parent.FirstChild; child != nil; child = child.NextSibling {
		if isZimbraDivider(child) && hasMeaningfulHTMLSibling(child) {
			divider = child
		}
	}
	return divider
}

func hasMeaningfulHTMLSibling(node *xhtml.Node) bool {
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == xhtml.CommentNode {
			continue
		}
		if sibling.Type == xhtml.TextNode && strings.TrimSpace(sibling.Data) == "" {
			continue
		}
		return true
	}
	return false
}

func lastMeaningfulHTMLChild(parent *xhtml.Node) *xhtml.Node {
	for child := parent.LastChild; child != nil; child = child.PrevSibling {
		if child.Type == xhtml.CommentNode {
			continue
		}
		if child.Type == xhtml.TextNode && strings.TrimSpace(child.Data) == "" {
			continue
		}
		return child
	}
	return nil
}

func htmlHasMeaningfulContent(node *xhtml.Node) bool {
	if node.Type == xhtml.TextNode {
		return strings.TrimSpace(node.Data) != ""
	}
	if node.Type == xhtml.CommentNode {
		return false
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if htmlHasMeaningfulContent(child) {
			return true
		}
	}
	return false
}

func isHTMLQuoteContainer(node *xhtml.Node) bool {
	if node.Type != xhtml.ElementNode {
		return false
	}
	class := htmlAttribute(node, "class")
	if hasHTMLClass(class, "gmail_quote") {
		return true
	}
	id := strings.ToLower(htmlAttribute(node, "id"))
	switch id {
	case "divrplyfwdmsg", "olk_src_body_section", "zmail_extra":
		return true
	}
	if isZimbraDivider(node) {
		return true
	}
	if node.Data == "div" && strings.Contains(strings.ToLower(htmlAttribute(node, "style")), "border-top") {
		return true
	}
	return node.Data == "blockquote" && strings.EqualFold(htmlAttribute(node, "type"), "cite")
}

func isHTMLQuoteCandidate(node *xhtml.Node) bool {
	if !isHTMLQuoteContainer(node) || isZimbraDivider(node) {
		return false
	}
	if node.Data == "div" && strings.Contains(strings.ToLower(htmlAttribute(node, "style")), "border-top") {
		return hasHTMLReplyHeaderBlock(node)
	}
	return true
}

func isZimbraDivider(node *xhtml.Node) bool {
	return node.Type == xhtml.ElementNode && node.Data == "hr" &&
		htmlAttribute(node, "data-marker") == "__DIVIDER__"
}

func hasHTMLReplyHeaderBlock(node *xhtml.Node) bool {
	return hasReplyHeaderBlock(splitReplyTextLines(htmlTextContent(node)), 0)
}

func htmlTextContent(node *xhtml.Node) string {
	var text strings.Builder
	var visit func(*xhtml.Node)
	visit = func(current *xhtml.Node) {
		switch current.Type {
		case xhtml.DocumentNode, xhtml.CommentNode, xhtml.DoctypeNode, xhtml.ErrorNode, xhtml.RawNode:
		case xhtml.TextNode:
			text.WriteString(current.Data)
		case xhtml.ElementNode:
			if current.Data == "br" {
				text.WriteByte('\n')
				return
			}
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
			switch current.Data {
			case "div", "li", "p", "tr":
				text.WriteByte('\n')
			}
		}
	}
	visit(node)
	return text.String()
}

func isHTMLQuoteAttribution(node *xhtml.Node) bool {
	if node == nil || node.Type != xhtml.ElementNode {
		return false
	}
	return hasHTMLClass(htmlAttribute(node, "class"), "moz-cite-prefix")
}

func previousMeaningfulHTMLSibling(node *xhtml.Node) *xhtml.Node {
	for sibling := node.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
		if sibling.Type == xhtml.CommentNode {
			continue
		}
		if sibling.Type == xhtml.TextNode && strings.TrimSpace(sibling.Data) == "" {
			continue
		}
		return sibling
	}
	return nil
}

func htmlAttribute(node *xhtml.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func hasHTMLClass(classes, want string) bool {
	for _, class := range strings.Fields(classes) {
		if strings.EqualFold(class, want) {
			return true
		}
	}
	return false
}

func htmlQuoteHasExplicitClose(body string, candidate *xhtml.Node) bool {
	type openTag struct {
		name       string
		quoteIndex int
	}

	targetIndex := htmlQuoteIndex(candidate)
	if targetIndex < 0 {
		return false
	}
	tokenizer := xhtml.NewTokenizer(strings.NewReader(body))
	open := make([]openTag, 0, 8)
	quoteClosed := make([]bool, 0, 2)
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case xhtml.ErrorToken:
			return targetIndex < len(quoteClosed) && quoteClosed[targetIndex]
		case xhtml.StartTagToken:
			token := tokenizer.Token()
			quoteIndex := -1
			if isHTMLQuoteToken(token) {
				quoteIndex = len(quoteClosed)
				quoteClosed = append(quoteClosed, isHTMLQuoteVoidToken(token))
			}
			open = append(open, openTag{name: token.Data, quoteIndex: quoteIndex})
		case xhtml.SelfClosingTagToken:
			token := tokenizer.Token()
			quoteIndex := -1
			if isHTMLQuoteToken(token) {
				quoteIndex = len(quoteClosed)
				quoteClosed = append(quoteClosed, true)
			}
			open = append(open, openTag{name: token.Data, quoteIndex: quoteIndex})
		case xhtml.EndTagToken:
			token := tokenizer.Token()
			for index := len(open) - 1; index >= 0; index-- {
				if strings.EqualFold(open[index].name, token.Data) {
					if open[index].quoteIndex >= 0 {
						quoteClosed[open[index].quoteIndex] = true
					}
					open = open[:index]
					break
				}
			}
		case xhtml.TextToken, xhtml.CommentToken, xhtml.DoctypeToken:
		}
	}
}

func htmlQuoteIndex(candidate *xhtml.Node) int {
	root := candidate
	for root.Parent != nil {
		root = root.Parent
	}

	index := -1
	count := 0
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if isHTMLQuoteContainer(node) {
			if node == candidate {
				index = count
			}
			count++
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return index
}

func isHTMLQuoteToken(token xhtml.Token) bool {
	class := ""
	id := ""
	typeAttr := ""
	for _, attr := range token.Attr {
		switch strings.ToLower(attr.Key) {
		case "class":
			class = attr.Val
		case "id":
			id = strings.ToLower(attr.Val)
		case "type":
			typeAttr = attr.Val
		}
	}
	if hasHTMLClass(class, "gmail_quote") {
		return true
	}
	switch id {
	case "divrplyfwdmsg", "olk_src_body_section", "zmail_extra":
		return true
	}
	if strings.EqualFold(token.Data, "hr") && htmlAttributeToken(token, "data-marker") == "__DIVIDER__" {
		return true
	}
	if strings.EqualFold(token.Data, "div") && strings.Contains(strings.ToLower(htmlAttributeToken(token, "style")), "border-top") {
		return true
	}
	return strings.EqualFold(token.Data, "blockquote") && strings.EqualFold(typeAttr, "cite")
}

func isHTMLQuoteVoidToken(token xhtml.Token) bool {
	return strings.EqualFold(token.Data, "hr") && htmlAttributeToken(token, "data-marker") == "__DIVIDER__"
}

func htmlAttributeToken(token xhtml.Token, name string) string {
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func isHTMLDocument(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "<!doctype") || strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<head") || strings.Contains(lower, "<body")
}
