package lsp

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	ppi "github.com/skaji/go-ppi"
	"github.com/skaji/perl-language-server/internal/analysis"
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
	s.logger.Debug("lsp server created", "name", lsName, "version", version)
	s.handler = protocol.Handler{
		Initialize:             s.initialize,
		Initialized:            s.initialized,
		Shutdown:               s.shutdown,
		SetTrace:               s.setTrace,
		TextDocumentDidOpen:    s.didOpen,
		TextDocumentDidChange:  s.didChange,
		TextDocumentDidClose:   s.didClose,
		TextDocumentHover:      s.hover,
		TextDocumentDefinition: s.definition,
		TextDocumentCompletion: s.completion,
	}
	return s
}

func (s *Server) RunStdio() error {
	s.logger.Info("starting stdio server", "name", lsName, "version", version)
	srv := server.NewServer(&s.handler, lsName, false)
	return srv.RunStdio()
}

type documentData struct {
	uri     string
	text    string
	version *protocol.UInteger
	parsed  *ppi.Document
	index   *analysis.Index
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
	index := analysis.IndexDocument(parsed)
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := &documentData{
		uri:     uri,
		text:    text,
		version: version,
		parsed:  parsed,
		index:   index,
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
	s.logger.Debug("initialize request")
	capabilities := s.handler.CreateServerCapabilities()

	syncKind := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: &protocol.True,
		Change:    &syncKind,
	}
	capabilities.HoverProvider = true
	capabilities.DefinitionProvider = true
	capabilities.CompletionProvider = &protocol.CompletionOptions{}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    lsName,
			Version: &version,
		},
	}, nil
}

func (s *Server) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	s.logger.Debug("initialized notification")
	return nil
}

func (s *Server) shutdown(_ *glsp.Context) error {
	s.logger.Debug("shutdown request")
	protocol.SetTraceValue(protocol.TraceValueOff)
	return nil
}

func (s *Server) setTrace(_ *glsp.Context, params *protocol.SetTraceParams) error {
	s.logger.Debug("setTrace request", "value", params.Value)
	protocol.SetTraceValue(params.Value)
	return nil
}

func (s *Server) didOpen(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.logger.Debug("didOpen", "uri", params.TextDocument.URI, "version", params.TextDocument.Version, "languageId", params.TextDocument.LanguageID)
	version := toUIntegerPtr(params.TextDocument.Version)
	doc := s.docs.set(string(params.TextDocument.URI), params.TextDocument.Text, version)
	s.publishDiagnostics(context, params.TextDocument.URI, doc)
	s.logger.Debug("document opened", "uri", params.TextDocument.URI, "errors", len(doc.parsed.Errors))
	return nil
}

func (s *Server) didChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.logger.Debug("didChange", "uri", params.TextDocument.URI, "version", params.TextDocument.Version, "changes", len(params.ContentChanges))
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
			s.logger.Debug("document changed", "uri", params.TextDocument.URI, "errors", len(doc.parsed.Errors))
			return nil
		case protocol.TextDocumentContentChangeEvent:
			if change.Range == nil {
				version := toUIntegerPtr(params.TextDocument.Version)
				doc := s.docs.set(uri, change.Text, version)
				s.publishDiagnostics(context, params.TextDocument.URI, doc)
				s.logger.Debug("document changed", "uri", params.TextDocument.URI, "errors", len(doc.parsed.Errors))
				return nil
			}
		}
	}
	return nil
}

func (s *Server) didClose(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.logger.Debug("didClose", "uri", params.TextDocument.URI)
	s.docs.delete(string(params.TextDocument.URI))
	s.publishDiagnostics(context, params.TextDocument.URI, nil)
	s.logger.Debug("document closed", "uri", params.TextDocument.URI)
	return nil
}

func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.logger.Debug("hover", "uri", params.TextDocument.URI, "line", params.Position.Line, "character", params.Position.Character)
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		s.logger.Debug("hover skipped: no document")
		return nil, nil
	}

	offset := params.Position.IndexIn(doc.text)
	token := tokenAtOffset(doc.parsed.Tokens, offset)
	if token == nil || isTriviaToken(token.Type) {
		s.logger.Debug("hover skipped: no token")
		return nil, nil
	}

	node := findStatementForOffset(doc.parsed.Root, offset)
	content := hoverContentForNode(node)
	if content == "" {
		content = fmt.Sprintf("%s: %s", token.Type, token.Value)
	}
	if content == "" {
		s.logger.Debug("hover skipped: empty content")
		return nil, nil
	}
	s.logger.Debug("hover resolved", "token", token.Value, "type", token.Type, "node", node.Kind)

	rng := tokenRange(doc.text, token)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &rng,
	}, nil
}

func (s *Server) definition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	s.logger.Debug("definition", "uri", params.TextDocument.URI, "line", params.Position.Line, "character", params.Position.Character)
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		s.logger.Debug("definition skipped: no document")
		return nil, nil
	}

	offset := params.Position.IndexIn(doc.text)
	token := tokenAtOffset(doc.parsed.Tokens, offset)
	if token == nil || isTriviaToken(token.Type) {
		s.logger.Debug("definition skipped: no token")
		return nil, nil
	}

	name := token.Value
	def := findDefinition(doc.parsed.Root, name)
	if def == nil {
		s.logger.Debug("definition not found", "name", name)
		return nil, nil
	}

	locRange, ok := nodeNameRange(doc.text, def)
	if !ok {
		s.logger.Debug("definition skipped: no range", "name", name)
		return nil, nil
	}

	loc := protocol.Location{
		URI:   params.TextDocument.URI,
		Range: locRange,
	}
	s.logger.Debug("definition resolved", "name", name, "kind", def.Kind)
	return []protocol.Location{loc}, nil
}

func (s *Server) completion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	s.logger.Debug("completion", "uri", params.TextDocument.URI, "line", params.Position.Line, "character", params.Position.Character)
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		s.logger.Debug("completion skipped: no document")
		return nil, nil
	}

	offset := params.Position.IndexIn(doc.text)
	prefix := completionPrefix(doc.text, offset)
	vars := []analysis.Symbol{}
	if doc.index != nil {
		vars = doc.index.VariablesAt(offset)
	}
	items := completionItems(doc.parsed, vars, prefix)
	s.logger.Debug("completion resolved", "prefix", prefix, "count", len(items))

	return protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
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

func tokenAtStart(tokens []ppi.Token, start int) *ppi.Token {
	for i := range tokens {
		tok := &tokens[i]
		if tok.Start == start {
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

func findDefinition(root *ppi.Node, name string) *ppi.Node {
	if root == nil || name == "" {
		return nil
	}
	var best *ppi.Node
	var walk func(n *ppi.Node)
	walk = func(n *ppi.Node) {
		if n == nil {
			return
		}
		if n.Type == ppi.NodeStatement {
			switch n.Kind {
			case "statement::sub", "statement::package":
				if n.Name == name {
					best = n
					return
				}
			}
		}
		for _, child := range n.Children {
			if best != nil {
				return
			}
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

func nodeNameRange(text string, node *ppi.Node) (protocol.Range, bool) {
	if node == nil || node.Name == "" {
		return protocol.Range{}, false
	}
	for i := range node.Tokens {
		tok := node.Tokens[i]
		if tok.Value == node.Name {
			return tokenRange(text, &tok), true
		}
	}
	start, _, ok := nodeTokenRange(node)
	if !ok {
		return protocol.Range{}, false
	}
	tok := tokenAtStart(node.Tokens, start)
	if tok == nil {
		return protocol.Range{}, false
	}
	return tokenRange(text, tok), true
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

func completionPrefix(text string, offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	start := offset
	for start > 0 {
		ch := text[start-1]
		if isCompletionChar(ch) {
			start--
			continue
		}
		break
	}
	return text[start:offset]
}

func isCompletionChar(ch byte) bool {
	switch {
	case ch >= 'a' && ch <= 'z':
		return true
	case ch >= 'A' && ch <= 'Z':
		return true
	case ch >= '0' && ch <= '9':
		return true
	case ch == '_' || ch == ':' || ch == '$' || ch == '@' || ch == '%':
		return true
	default:
		return false
	}
}

func completionItems(doc *ppi.Document, vars []analysis.Symbol, prefix string) []protocol.CompletionItem {
	if doc == nil || doc.Root == nil {
		return nil
	}
	items := make([]protocol.CompletionItem, 0)
	seen := make(map[string]protocol.CompletionItemKind)

	add := func(label string, kind protocol.CompletionItemKind, detail string) {
		if label == "" {
			return
		}
		if !strings.HasPrefix(label, prefix) {
			return
		}
		if _, ok := seen[label]; ok {
			return
		}
		seen[label] = kind
		k := kind
		d := detail
		items = append(items, protocol.CompletionItem{
			Label:  label,
			Kind:   &k,
			Detail: &d,
		})
	}

	for _, kw := range perlKeywords() {
		add(kw, protocol.CompletionItemKindKeyword, "keyword")
	}
	for _, fn := range perlBuiltins() {
		add(fn, protocol.CompletionItemKindFunction, "builtin")
	}

	for _, sym := range vars {
		detail := "var"
		if sym.Storage != "" {
			detail = sym.Storage + " var"
		}
		add(sym.Name, protocol.CompletionItemKindVariable, detail)
	}

	walkNodes(doc.Root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement {
			return
		}
		switch n.Kind {
		case "statement::sub":
			add(n.Name, protocol.CompletionItemKindFunction, "sub")
		case "statement::package":
			add(n.Name, protocol.CompletionItemKindModule, "package")
		}
	})

	return items
}

func perlKeywords() []string {
	return []string{
		"sub", "package", "use", "require", "my", "our", "state", "local",
		"if", "elsif", "else", "unless", "while", "until", "for", "foreach",
		"given", "when", "default", "continue", "do", "eval",
		"last", "next", "redo", "goto", "return",
		"BEGIN", "CHECK", "INIT", "END",
	}
}

func perlBuiltins() []string {
	return []string{
		"abs", "accept", "alarm", "atan2", "bind", "binmode", "bless", "caller",
		"chdir", "chmod", "chomp", "chop", "chown", "chr", "chroot", "close",
		"closedir", "connect", "cos", "crypt", "dbmclose", "dbmopen", "defined",
		"delete", "die", "do", "dump", "each", "endgrent", "endhostent",
		"endnetent", "endprotoent", "endpwent", "endservent", "eof", "eval",
		"exec", "exists", "exit", "exp", "fcntl", "fileno", "flock", "fork",
		"format", "formline", "getc", "getgrent", "getgrgid", "getgrnam",
		"gethostbyaddr", "gethostbyname", "gethostent", "getlogin",
		"getnetbyaddr", "getnetbyname", "getnetent", "getpeername",
		"getpgrp", "getppid", "getpriority", "getprotobyname",
		"getprotobynumber", "getprotoent", "getpwent", "getpwnam",
		"getpwuid", "getservbyname", "getservbyport", "getservent",
		"getsockname", "getsockopt", "glob", "gmtime", "goto", "grep",
		"hex", "index", "int", "ioctl", "join", "keys", "kill", "last",
		"lc", "lcfirst", "length", "link", "listen", "local", "localtime",
		"log", "lstat", "map", "mkdir", "msgctl", "msgget", "msgrcv",
		"msgsnd", "my", "next", "oct", "open", "opendir", "ord", "pack",
		"pipe", "pop", "pos", "print", "printf", "prototype", "push",
		"quotemeta", "rand", "read", "readdir", "readline", "readlink",
		"readpipe", "recv", "redo", "ref", "rename", "require", "reset",
		"return", "reverse", "rewinddir", "rindex", "rmdir", "say", "scalar",
		"seek", "seekdir", "select", "semctl", "semget", "semop", "send",
		"setgrent", "sethostent", "setnetent", "setpgrp", "setpriority",
		"setprotoent", "setpwent", "setservent", "setsockopt", "shift",
		"shmctl", "shmget", "shmread", "shmwrite", "shutdown", "sin",
		"sleep", "socket", "socketpair", "sort", "splice", "split", "sprintf",
		"srand", "stat", "state", "study", "substr", "symlink", "syscall",
		"sysopen", "sysread", "sysseek", "system", "syswrite", "tell",
		"telldir", "tie", "tied", "time", "times", "truncate", "uc",
		"ucfirst", "umask", "undef", "unlink", "unpack", "unshift", "untie",
		"utime", "values", "vec", "wait", "waitpid", "wantarray", "warn",
		"write",
	}
}

func walkNodes(node *ppi.Node, fn func(*ppi.Node)) {
	if node == nil {
		return
	}
	fn(node)
	for _, child := range node.Children {
		walkNodes(child, fn)
	}
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
	s.logger.Debug("diagnostics published", "uri", uri, "count", len(diagnostics), "version", version)

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
