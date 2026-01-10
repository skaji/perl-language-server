package lsp

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	ppi "github.com/skaji/go-ppi"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

const lsName = "perl-language-server"

var version = "0.0.1"

type Server struct {
	handler protocol.Handler
	docs    *documentStore
	logger  *slog.Logger
}

func NewServer(logger *slog.Logger) *Server {
	s := &Server{
		docs:   newDocumentStore(),
		logger: logger,
	}
	s.handler = protocol.Handler{
		Initialize:            s.initialize,
		Initialized:           s.initialized,
		Shutdown:              s.shutdown,
		SetTrace:              s.setTrace,
		TextDocumentDidOpen:   s.didOpen,
		TextDocumentDidChange: s.didChange,
		TextDocumentDidClose:  s.didClose,
		TextDocumentHover:     s.hover,
	}
	return s
}

func (s *Server) RunStdio() error {
	srv := server.NewServer(&s.handler, lsName, false)
	return srv.RunStdio()
}

type documentData struct {
	uri     string
	text    string
	version *protocol.UInteger
	parsed  *ppi.Document
}

type documentStore struct {
	mu   sync.RWMutex
	docs map[string]*documentData
}

func newDocumentStore() *documentStore {
	return &documentStore{docs: make(map[string]*documentData)}
}

func (s *documentStore) set(uri string, text string, version *protocol.UInteger) *documentData {
	parsed := parseDocument(text)
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := &documentData{
		uri:     uri,
		text:    text,
		version: version,
		parsed:  parsed,
	}
	s.docs[uri] = doc
	return doc
}

func (s *documentStore) get(uri string) (*documentData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[uri]
	return doc, ok
}

func (s *documentStore) delete(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

func parseDocument(text string) *ppi.Document {
	doc := ppi.NewDocument(text)
	doc.ParseWithDiagnostics()
	return doc
}

func (s *Server) initialize(_ *glsp.Context, _ *protocol.InitializeParams) (any, error) {
	capabilities := s.handler.CreateServerCapabilities()

	syncKind := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: &protocol.True,
		Change:    &syncKind,
	}
	capabilities.HoverProvider = true

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    lsName,
			Version: &version,
		},
	}, nil
}

func (s *Server) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	return nil
}

func (s *Server) shutdown(_ *glsp.Context) error {
	protocol.SetTraceValue(protocol.TraceValueOff)
	return nil
}

func (s *Server) setTrace(_ *glsp.Context, params *protocol.SetTraceParams) error {
	protocol.SetTraceValue(params.Value)
	return nil
}

func (s *Server) didOpen(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	version := toUIntegerPtr(params.TextDocument.Version)
	doc := s.docs.set(string(params.TextDocument.URI), params.TextDocument.Text, version)
	s.publishDiagnostics(context, params.TextDocument.URI, doc)
	s.logger.Debug("document opened", "uri", params.TextDocument.URI)
	return nil
}

func (s *Server) didChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}
	uri := string(params.TextDocument.URI)
	for i := len(params.ContentChanges) - 1; i >= 0; i-- {
		switch change := params.ContentChanges[i].(type) {
		case protocol.TextDocumentContentChangeEventWhole:
			version := toUIntegerPtr(params.TextDocument.Version)
			doc := s.docs.set(uri, change.Text, version)
			s.publishDiagnostics(context, params.TextDocument.URI, doc)
			s.logger.Debug("document changed", "uri", params.TextDocument.URI)
			return nil
		case protocol.TextDocumentContentChangeEvent:
			if change.Range == nil {
				version := toUIntegerPtr(params.TextDocument.Version)
				doc := s.docs.set(uri, change.Text, version)
				s.publishDiagnostics(context, params.TextDocument.URI, doc)
				s.logger.Debug("document changed", "uri", params.TextDocument.URI)
				return nil
			}
		}
	}
	return nil
}

func (s *Server) didClose(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.docs.delete(string(params.TextDocument.URI))
	s.publishDiagnostics(context, params.TextDocument.URI, nil)
	s.logger.Debug("document closed", "uri", params.TextDocument.URI)
	return nil
}

func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		return nil, nil
	}

	offset := params.Position.IndexIn(doc.text)
	token := tokenAtOffset(doc.parsed.Tokens, offset)
	if token == nil || isTriviaToken(token.Type) {
		return nil, nil
	}

	node := findStatementForOffset(doc.parsed.Root, offset)
	content := hoverContentForNode(node)
	if content == "" {
		content = fmt.Sprintf("%s: %s", token.Type, token.Value)
	}
	if content == "" {
		return nil, nil
	}

	rng := tokenRange(doc.text, token)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &rng,
	}, nil
}

func tokenAtOffset(tokens []ppi.Token, offset int) *ppi.Token {
	for i := range tokens {
		tok := &tokens[i]
		if offset >= tok.Start && offset < tok.End {
			return tok
		}
	}
	return nil
}

func isTriviaToken(t ppi.TokenType) bool {
	switch t {
	case ppi.TokenWhitespace, ppi.TokenComment, ppi.TokenEnd:
		return true
	default:
		return false
	}
}

func findStatementForOffset(root *ppi.Node, offset int) *ppi.Node {
	var best *ppi.Node
	bestLen := 0
	var walk func(n *ppi.Node)
	walk = func(n *ppi.Node) {
		if n == nil {
			return
		}
		start, end, ok := nodeTokenRange(n)
		if ok && offset >= start && offset < end && n.Type == ppi.NodeStatement {
			if best == nil || (end-start) < bestLen {
				best = n
				bestLen = end - start
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	return best
}

func nodeTokenRange(n *ppi.Node) (int, int, bool) {
	if n == nil {
		return 0, 0, false
	}
	found := false
	start := 0
	end := 0
	for i := range n.Tokens {
		tok := n.Tokens[i]
		if !found {
			start, end, found = tok.Start, tok.End, true
			continue
		}
		if tok.Start < start {
			start = tok.Start
		}
		if tok.End > end {
			end = tok.End
		}
	}
	for _, child := range n.Children {
		cs, ce, ok := nodeTokenRange(child)
		if !ok {
			continue
		}
		if !found {
			start, end, found = cs, ce, true
			continue
		}
		if cs < start {
			start = cs
		}
		if ce > end {
			end = ce
		}
	}
	return start, end, found
}

func hoverContentForNode(node *ppi.Node) string {
	if node == nil {
		return ""
	}

	switch node.Kind {
	case "statement::package":
		header := strings.TrimSpace(strings.Join([]string{node.Keyword, node.Name}, " "))
		var lines []string
		if header != "" {
			lines = append(lines, "```perl", header+";", "```")
		}
		if node.PackageVersion != "" {
			lines = append(lines, "version: "+node.PackageVersion)
		}
		return strings.Join(lines, "\n")
	case "statement::sub":
		var sigParts []string
		if node.Name != "" {
			sigParts = append(sigParts, node.Name)
		}
		if node.SubSignature != "" {
			sigParts = append(sigParts, node.SubSignature)
		}
		if node.SubPrototype != "" {
			sigParts = append(sigParts, node.SubPrototype)
		}
		line := "sub"
		if len(sigParts) > 0 {
			line = line + " " + strings.Join(sigParts, " ")
		}
		lines := []string{"```perl", line, "```"}
		if node.SubReserved {
			lines = append(lines, "reserved: true")
		}
		if attrs := formatAttributes(node.AttrMeta); attrs != "" {
			lines = append(lines, "attributes: "+attrs)
		}
		return strings.Join(lines, "\n")
	case "statement::include":
		var parts []string
		if node.Keyword != "" {
			parts = append(parts, node.Keyword)
		}
		if node.Name != "" {
			parts = append(parts, node.Name)
		}
		if node.Version != "" {
			parts = append(parts, node.Version)
		}
		line := strings.Join(parts, " ")
		lines := []string{"```perl", line + ";", "```"}
		if len(node.ImportItems) > 0 {
			lines = append(lines, "imports: "+strings.Join(node.ImportItems, ", "))
		}
		return strings.Join(lines, "\n")
	case "statement::scheduled":
		if node.Keyword == "" {
			return ""
		}
		lines := []string{"```perl", node.Keyword + " { ... }", "```"}
		return strings.Join(lines, "\n")
	case "statement::control":
		line := strings.TrimSpace(strings.Join([]string{node.Keyword, tokensToString(node.Header)}, " "))
		if line == "" {
			return ""
		}
		lines := []string{"```perl", line, "```"}
		if node.IterVar != "" {
			lines = append(lines, "iter: "+node.IterVar)
		}
		if node.LoopKind != "" {
			lines = append(lines, "loop: "+node.LoopKind)
		}
		if node.LoopKind == "cstyle" {
			init := tokensToString(node.HeaderInit)
			cond := tokensToString(node.HeaderCond)
			step := tokensToString(node.HeaderStep)
			lines = append(lines, fmt.Sprintf("cstyle: %s; %s; %s", init, cond, step))
		}
		return strings.Join(lines, "\n")
	case "statement::postfix":
		line := strings.TrimSpace(strings.Join([]string{node.Keyword, tokensToString(node.Header)}, " "))
		if line == "" {
			return ""
		}
		return strings.Join([]string{"```perl", line, "```"}, "\n")
	case "statement::label":
		if node.Name == "" {
			return ""
		}
		return strings.Join([]string{"```perl", node.Name + ":", "```"}, "\n")
	default:
		return ""
	}
}

func formatAttributes(attrs []ppi.AttrMeta) string {
	if len(attrs) == 0 {
		return ""
	}
	out := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		if attr.Args != "" {
			out = append(out, fmt.Sprintf("%s(%s)", attr.Name, attr.Args))
		} else {
			out = append(out, attr.Name)
		}
	}
	return strings.Join(out, ", ")
}

func tokensToString(tokens []ppi.Token) string {
	var b strings.Builder
	prev := ""
	for _, tok := range tokens {
		if isTriviaToken(tok.Type) {
			continue
		}
		if b.Len() > 0 && needsSpace(prev, tok.Value) {
			b.WriteByte(' ')
		}
		b.WriteString(tok.Value)
		prev = tok.Value
	}
	return b.String()
}

func needsSpace(prev string, cur string) bool {
	if prev == "" {
		return false
	}
	switch cur {
	case ")", "]", "}", ",", ";", "->":
		return false
	}
	switch prev {
	case "(", "[", "{", "->":
		return false
	}
	return true
}

func (s *Server) publishDiagnostics(context *glsp.Context, uri protocol.DocumentUri, doc *documentData) {
	var diagnostics []protocol.Diagnostic
	var version *protocol.UInteger
	if doc != nil {
		version = doc.version
		diagnostics = toProtocolDiagnostics(doc.text, doc.parsed)
	}

	if context != nil && context.Notify != nil {
		context.Notify(protocol.ServerTextDocumentPublishDiagnostics, &protocol.PublishDiagnosticsParams{
			URI:         uri,
			Version:     version,
			Diagnostics: diagnostics,
		})
	}
	s.logger.Debug("diagnostics published", "uri", uri, "count", len(diagnostics))

	_ = protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     version,
		Diagnostics: diagnostics,
	}
}

func toProtocolDiagnostics(text string, doc *ppi.Document) []protocol.Diagnostic {
	if doc == nil {
		return nil
	}
	var out []protocol.Diagnostic
	for _, diag := range doc.Errors {
		sev := toProtocolSeverity(diag.Severity)
		msg := diag.Message
		source := "go-ppi"
		rng := diagnosticRange(text, diag.Offset)
		out = append(out, protocol.Diagnostic{
			Range:    rng,
			Severity: &sev,
			Source:   &source,
			Message:  msg,
		})
	}
	return out
}

func toProtocolSeverity(sev ppi.DiagnosticSeverity) protocol.DiagnosticSeverity {
	switch sev {
	case ppi.SeverityWarning:
		return protocol.DiagnosticSeverityWarning
	default:
		return protocol.DiagnosticSeverityError
	}
}

func diagnosticRange(text string, offset int) protocol.Range {
	start := positionFromOffset(text, offset)
	endOffset := offset
	if endOffset < len(text) {
		endOffset++
	}
	end := positionFromOffset(text, endOffset)
	return protocol.Range{Start: start, End: end}
}

func toUIntegerPtr(version protocol.Integer) *protocol.UInteger {
	if version < 0 {
		return nil
	}
	u := protocol.UInteger(version)
	return &u
}

func tokenRange(text string, token *ppi.Token) protocol.Range {
	start := positionFromOffset(text, token.Start)
	end := positionFromOffset(text, token.End)
	return protocol.Range{Start: start, End: end}
}

func positionFromOffset(text string, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line := protocol.UInteger(0)
	lineStart := 0
	for i, r := range text {
		if i >= offset {
			break
		}
		if r == '\n' {
			line++
			lineStart = i + 1
		}
	}
	var character protocol.UInteger
	for _, r := range text[lineStart:offset] {
		if r >= 0x10000 {
			character += 2
		} else {
			character++
		}
	}
	return protocol.Position{Line: line, Character: character}
}
