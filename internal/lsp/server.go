package lsp

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

	workspaceMu    sync.RWMutex
	workspaceRoots []string
	incRoots       []string
	extraRoots     map[string]struct{}
	workspaceIndex *analysis.WorkspaceIndex
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

func (s *Server) initialize(_ *glsp.Context, params *protocol.InitializeParams) (any, error) {
	s.logger.Debug("initialize request")
	s.initWorkspaceIndex(params)
	capabilities := s.handler.CreateServerCapabilities()

	syncKind := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: &protocol.True,
		Change:    &syncKind,
	}
	capabilities.HoverProvider = true
	capabilities.DefinitionProvider = true
	capabilities.CompletionProvider = &protocol.CompletionOptions{
		TriggerCharacters: []string{"$", "@", "%", ">"},
	}

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
	s.logger.Debug("hover", "uri", params.TextDocument.URI, "line", params.Position.Line+1, "character", params.Position.Character+1)
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		s.logger.Debug("hover skipped: no document")
		return nil, nil
	}

	offset := params.Position.IndexIn(doc.text)
	tokenIdx := tokenIndexAtOffset(doc.parsed.Tokens, offset)
	if tokenIdx < 0 {
		s.logger.Debug("hover skipped: no token")
		return nil, nil
	}
	token := doc.parsed.Tokens[tokenIdx]
	if isTriviaToken(token.Type) {
		s.logger.Debug("hover skipped: no token")
		return nil, nil
	}

	node := findStatementForOffset(doc.parsed.Root, offset)
	content := ""
	if token.Type == ppi.TokenSymbol {
		if sigType := hoverVarSigType(doc, tokenIdx, offset); sigType != "" {
			content = "type: " + sigType
		}
	}
	if content == "" {
		content = hoverContentForNode(node)
	}
	if content == "" {
		content = fmt.Sprintf("%s: %s", token.Type, token.Value)
	}
	if content == "" {
		s.logger.Debug("hover skipped: empty content")
		return nil, nil
	}
	s.logger.Debug("hover resolved", "token", token.Value, "type", token.Type, "node", node.Kind)

	rng := tokenRange(doc.text, &token)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
		Range: &rng,
	}, nil
}

func hoverVarSigType(doc *documentData, tokenIdx int, offset int) string {
	if doc == nil || doc.parsed == nil || doc.index == nil {
		return ""
	}
	tok := doc.parsed.Tokens[tokenIdx]
	if tok.Type != ppi.TokenSymbol {
		return ""
	}
	if tok.Value == "" {
		return ""
	}
	switch tok.Value[0] {
	case '$', '@', '%':
	default:
		return ""
	}
	decl := findVarDeclSymbol(doc.index.VariablesAt(offset), tok.Value)
	if decl == nil {
		return ""
	}
	sig := sigCommentBeforeOffset(doc.text, decl.Start)
	if sig == "" {
		return ""
	}
	if strings.Contains(sig, "->") {
		return ""
	}
	return normalizeVarSigType(sig, tok.Value)
}

func findVarDeclSymbol(vars []analysis.Symbol, name string) *analysis.Symbol {
	var best *analysis.Symbol
	for i := range vars {
		sym := &vars[i]
		if sym.Kind != analysis.SymbolVar || sym.Name != name {
			continue
		}
		if best == nil || sym.Start > best.Start {
			best = sym
		}
	}
	return best
}

func sigCommentBeforeOffset(text string, offset int) string {
	lineStart, _ := lineBounds(text, offset)
	if lineStart <= 0 {
		return ""
	}
	prevEnd := lineStart - 1
	if prevEnd < 0 {
		return ""
	}
	prevStart := 0
	if idx := strings.LastIndexByte(text[:prevEnd], '\n'); idx >= 0 {
		prevStart = idx + 1
	}
	line := strings.TrimSpace(text[prevStart:prevEnd])
	if !strings.HasPrefix(line, "#") {
		return ""
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	if !strings.HasPrefix(line, ":SIG") {
		return ""
	}
	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close < open+1 {
		return ""
	}
	return strings.TrimSpace(line[open+1 : close])
}

func lineBounds(text string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	start := 0
	if idx := strings.LastIndexByte(text[:offset], '\n'); idx >= 0 {
		start = idx + 1
	}
	end := len(text)
	if idx := strings.IndexByte(text[offset:], '\n'); idx >= 0 {
		end = offset + idx
	}
	return start, end
}

func normalizeVarSigType(sig string, varName string) string {
	s := strings.TrimSpace(sig)
	if s == "" {
		return ""
	}
	return s
}

func (s *Server) definition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	s.logger.Debug("definition", "uri", params.TextDocument.URI, "line", params.Position.Line+1, "character", params.Position.Character+1)
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		s.logger.Debug("definition skipped: no document")
		return nil, nil
	}
	if path, ok := uriToPath(params.TextDocument.URI); ok {
		s.ensureUseLibPaths(doc.parsed.Root, path)
	}

	offset := params.Position.IndexIn(doc.text)
	tokenIdx := tokenIndexAtOffset(doc.parsed.Tokens, offset)
	if tokenIdx < 0 {
		s.logger.Debug("definition skipped: no token")
		return nil, nil
	}
	token := doc.parsed.Tokens[tokenIdx]
	if isTriviaToken(token.Type) {
		s.logger.Debug("definition skipped: no token")
		return nil, nil
	}
	if token.Type != ppi.TokenWord {
		s.logger.Debug("definition skipped: non-word token", "token", token.Value, "type", token.Type)
		return nil, nil
	}

	name, qualified := qualifiedNameAt(doc.parsed.Tokens, tokenIdx)
	def := findDefinition(doc.parsed.Root, name)
	if def == nil {
		if loc, ok := s.moduleLocation(name, params.TextDocument.URI); ok {
			s.logger.Debug("definition resolved (module)", "name", name)
			return []protocol.Location{loc}, nil
		}
		pkg := doc.parsed.PackageAt(offset)
		useImports := collectUseImports(doc.parsed.Root)
		defs, err := s.findWorkspaceDefinitions(name, params.TextDocument.URI, pkg, useImports, qualified)
		if err != nil {
			s.logger.Debug("definition lookup failed", "name", name, "error", err)
			return nil, nil
		}
		if len(defs) == 0 {
			s.logger.Debug("definition not found", "name", name)
			return nil, nil
		}
		locations := make([]protocol.Location, 0, len(defs))
		for _, def := range defs {
			rng, ok := rangeFromFile(def.File, def.Start, def.End)
			if !ok {
				continue
			}
			locations = append(locations, protocol.Location{
				URI:   protocol.DocumentUri(fileURI(def.File)),
				Range: rng,
			})
		}
		if len(locations) == 0 {
			s.logger.Debug("definition skipped: no ranges", "name", name)
			return nil, nil
		}
		s.logger.Debug("definition resolved (workspace)", "name", name, "count", len(locations))
		return locations, nil
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
	s.logger.Debug("completion", "uri", params.TextDocument.URI, "line", params.Position.Line+1, "character", params.Position.Character+1)
	doc, ok := s.docs.get(string(params.TextDocument.URI))
	if !ok || doc.parsed == nil {
		s.logger.Debug("completion skipped: no document")
		return nil, nil
	}

	offset := params.Position.IndexIn(doc.text)
	receivers := map[string]struct{}{}
	if doc.index != nil {
		if names := doc.index.ReceiverNamesAt(offset); names != nil {
			receivers = names
		}
	}
	if methodPrefix, start, recv, ok := methodPrefixForReceivers(doc.text, offset, receivers); ok {
		pkg := doc.parsed.PackageAt(offset)
		methods := methodsForPackage(doc.parsed, pkg)
		items := methodCompletionItems(methods, methodPrefix, doc.text, start, offset)
		s.logger.Debug("completion resolved", "prefix", methodPrefix, "count", len(items), "method", true, "package", pkg, "receiver", recv)
		return protocol.CompletionList{
			IsIncomplete: false,
			Items:        items,
		}, nil
	}

	prefix := completionPrefix(doc.text, offset)
	vars := []analysis.Symbol{}
	if doc.index != nil {
		vars = doc.index.VariablesAt(offset)
	}
	var replaceRange *protocol.Range
	if len(prefix) > 0 {
		start := max(0, offset-len(prefix))
		rng := protocol.Range{
			Start: positionFromOffset(doc.text, start),
			End:   positionFromOffset(doc.text, offset),
		}
		replaceRange = &rng
	}
	items := completionItems(doc.parsed, vars, prefix, replaceRange)
	s.logger.Debug("completion resolved", "prefix", prefix, "count", len(items))

	return protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

func tokenIndexAtOffset(tokens []ppi.Token, offset int) int {
	for i := range tokens {
		tok := &tokens[i]
		if offset >= tok.Start && offset < tok.End {
			return i
		}
	}
	return -1
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

func qualifiedNameAt(tokens []ppi.Token, idx int) (string, bool) {
	if idx < 0 || idx >= len(tokens) {
		return "", false
	}
	token := tokens[idx]
	if token.Type != ppi.TokenWord {
		return token.Value, false
	}
	if strings.Contains(token.Value, "::") {
		return token.Value, true
	}
	parts := []string{token.Value}
	for i := idx - 1; i >= 1; i -= 2 {
		if tokens[i].Type == ppi.TokenOperator && tokens[i].Value == "::" && tokens[i-1].Type == ppi.TokenWord {
			parts = append([]string{tokens[i-1].Value}, parts...)
			continue
		}
		break
	}
	for i := idx + 1; i+1 < len(tokens); i += 2 {
		if tokens[i].Type == ppi.TokenOperator && tokens[i].Value == "::" && tokens[i+1].Type == ppi.TokenWord {
			parts = append(parts, tokens[i+1].Value)
			continue
		}
		break
	}
	if len(parts) == 1 {
		return parts[0], false
	}
	return strings.Join(parts, "::"), true
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

func collectUseImports(root *ppi.Node) map[string]map[string]struct{} {
	if root == nil {
		return nil
	}
	out := make(map[string]map[string]struct{})
	walkNodes(root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::include" {
			return
		}
		if strings.ToLower(n.Keyword) != "use" {
			return
		}
		if n.Name == "" {
			return
		}
		items := n.ImportItems
		if len(items) == 0 {
			items = importItemsFromArgs(n.Args)
		}
		if len(items) == 0 {
			return
		}
		set := out[n.Name]
		if set == nil {
			set = make(map[string]struct{})
			out[n.Name] = set
		}
		for _, item := range items {
			name := normalizeImportName(item)
			if name == "" {
				continue
			}
			set[name] = struct{}{}
		}
	})
	return out
}

func collectUseSigilImports(root *ppi.Node) (map[string]map[string]struct{}, map[string]bool) {
	if root == nil {
		return nil, nil
	}
	imports := make(map[string]map[string]struct{})
	explicit := make(map[string]bool)
	walkNodes(root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::include" {
			return
		}
		if strings.ToLower(n.Keyword) != "use" {
			return
		}
		if n.Name == "" {
			return
		}
		if n.ImportKind != "" {
			explicit[n.Name] = true
		}
		if len(n.ImportList) == 0 {
			return
		}
		items := importSigilItems(n.ImportList, n.ImportKind)
		if len(items) == 0 {
			return
		}
		set := imports[n.Name]
		if set == nil {
			set = make(map[string]struct{})
			imports[n.Name] = set
		}
		for _, item := range items {
			set[item] = struct{}{}
		}
	})
	if len(imports) == 0 {
		imports = nil
	}
	if len(explicit) == 0 {
		explicit = nil
	}
	return imports, explicit
}

func importSigilItems(tokens []ppi.Token, kind string) []string {
	if len(tokens) == 0 {
		return nil
	}
	if kind == "qw" && len(tokens) == 1 && tokens[0].Type == ppi.TokenQuoteLike {
		return sigilsFromQW(tokens[0].Value)
	}
	var items []string
	for _, tok := range tokens {
		switch tok.Type {
		case ppi.TokenSymbol:
			if len(tok.Value) > 1 && (strings.HasPrefix(tok.Value, "$") || strings.HasPrefix(tok.Value, "@") || strings.HasPrefix(tok.Value, "%")) {
				items = append(items, tok.Value)
			}
		case ppi.TokenQuote:
			val := strings.Trim(tok.Value, "\"'`")
			if len(val) > 1 && (strings.HasPrefix(val, "$") || strings.HasPrefix(val, "@") || strings.HasPrefix(val, "%")) {
				items = append(items, val)
			}
		}
	}
	return items
}

func sigilsFromQW(value string) []string {
	if !strings.HasPrefix(value, "qw") || len(value) < 3 {
		return nil
	}
	body := value[2:]
	open := body[0]
	close := matchingDelimiter(open)
	if close == 0 {
		return nil
	}
	content := body[1:]
	if idx := strings.LastIndexByte(content, close); idx >= 0 {
		content = content[:idx]
	}
	if content == "" {
		return nil
	}
	var items []string
	for _, field := range strings.Fields(content) {
		if len(field) > 1 && (strings.HasPrefix(field, "$") || strings.HasPrefix(field, "@") || strings.HasPrefix(field, "%")) {
			items = append(items, field)
		}
	}
	return items
}

func matchingDelimiter(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	default:
		return open
	}
}

func collectUseModules(root *ppi.Node) map[string]struct{} {
	if root == nil {
		return nil
	}
	out := make(map[string]struct{})
	walkNodes(root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::include" {
			return
		}
		if strings.ToLower(n.Keyword) != "use" {
			return
		}
		if n.Name == "" {
			return
		}
		switch n.Name {
		case "strict", "warnings", "lib", "feature", "utf8", "parent", "base":
			return
		}
		out[n.Name] = struct{}{}
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeImportName(item string) string {
	if item == "" {
		return ""
	}
	item = strings.TrimPrefix(item, "&")
	item = strings.Trim(item, "\"'`")
	if item == "" {
		return ""
	}
	if idx := strings.LastIndex(item, "::"); idx >= 0 {
		return item[idx+2:]
	}
	return item
}

func importItemsFromArgs(tokens []ppi.Token) []string {
	if len(tokens) == 0 {
		return nil
	}
	var items []string
	for _, tok := range tokens {
		switch tok.Type {
		case ppi.TokenWord:
			items = append(items, tok.Value)
		case ppi.TokenQuote:
			name := strings.Trim(tok.Value, "\"'`")
			if name != "" {
				items = append(items, name)
			}
		}
	}
	return items
}

func filterUseImports(index *analysis.WorkspaceIndex, imports map[string]map[string]struct{}, exclude string) map[string]map[string]struct{} {
	if index == nil || len(imports) == 0 {
		return nil
	}
	out := make(map[string]map[string]struct{})
	for name, symbols := range imports {
		if name == "" || len(symbols) == 0 {
			continue
		}
		if len(index.FindPackages(name, exclude)) == 0 {
			continue
		}
		out[name] = symbols
	}
	return out
}

func (s *Server) exportedStrictVars(doc *ppi.Document, filePath string) map[string]struct{} {
	return s.exportedStrictVarsWithBase(doc, filePath, "")
}

func (s *Server) exportedStrictVarsWithBase(doc *ppi.Document, filePath, baseDir string) map[string]struct{} {
	if doc == nil || doc.Root == nil || filePath == "" {
		return nil
	}
	useModules := collectUseModules(doc.Root)
	if len(useModules) == 0 {
		return nil
	}
	useImports, explicitImports := collectUseSigilImports(doc.Root)
	useNames := collectUseImports(doc.Root)
	searchPaths := s.moduleSearchPathsWithBase(doc.Root, filePath, baseDir)
	if len(searchPaths) == 0 {
		return nil
	}
	exports := make(map[string]struct{})
	for name, items := range useImports {
		if len(items) == 0 {
			continue
		}
		if explicitImports != nil {
			if explicitImports[name] {
				for sym := range items {
					exports[sym] = struct{}{}
				}
			}
		}
	}
	for name := range useModules {
		if explicitImports != nil && explicitImports[name] {
			if !hasDefaultTag(useNames[name]) {
				continue
			}
		}
		modPath := findModuleFile(name, searchPaths)
		if modPath == "" {
			s.logger.Debug("module export lookup failed", "name", name)
			continue
		}
		src, err := os.ReadFile(modPath)
		if err != nil {
			s.logger.Debug("module export read failed", "name", name, "error", err)
			continue
		}
		modDoc := ppi.NewDocument(string(src))
		modDoc.ParseWithDiagnostics()
		exp := analysis.ExportedSymbols(modDoc)
		if len(exp) == 0 {
			continue
		}
		s.logger.Debug("module exports loaded", "name", name, "file", modPath, "count", len(exp))
		for sym := range exp {
			exports[sym] = struct{}{}
		}
	}
	if len(exports) == 0 {
		return nil
	}
	return exports
}

func (s *Server) moduleSearchPaths(root *ppi.Node, filePath string) []string {
	return s.moduleSearchPathsWithBase(root, filePath, "")
}

func (s *Server) moduleSearchPathsWithBase(root *ppi.Node, filePath, baseDir string) []string {
	paths := collectUseLibPathsWithBase(root, filePath, baseDir)
	base := baseDir
	if base == "" {
		base = filepath.Dir(filePath)
	}
	paths = append(paths, filepath.Join(base, "lib"))
	paths = append(paths, filepath.Join(base, "local", "lib", "perl5"))
	s.workspaceMu.RLock()
	incRoots := append([]string{}, s.incRoots...)
	s.workspaceMu.RUnlock()
	if len(incRoots) == 0 {
		roots, err := perlINCPaths()
		if err != nil {
			s.logger.Debug("perl @INC lookup failed", "error", err)
		} else {
			incRoots = roots
		}
	}
	paths = append(paths, incRoots...)
	paths = filterExistingRoots(paths, s.logger)
	return uniqueStrings(paths)
}

func findModuleFile(name string, roots []string) string {
	if name == "" || len(roots) == 0 {
		return ""
	}
	rel := strings.ReplaceAll(name, "::", string(os.PathSeparator)) + ".pm"
	for _, root := range roots {
		if root == "" {
			continue
		}
		path := filepath.Join(root, rel)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func hasDefaultTag(imports map[string]struct{}) bool {
	if len(imports) == 0 {
		return false
	}
	for name := range imports {
		switch strings.ToLower(name) {
		case ":default":
			return true
		}
	}
	return false
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

func completionItems(doc *ppi.Document, vars []analysis.Symbol, prefix string, replaceRange *protocol.Range) []protocol.CompletionItem {
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
		var insertText *string
		var textEdit *protocol.TextEdit
		if len(prefix) == 1 {
			sigil := prefix[:1]
			if (sigil == "$" || sigil == "@" || sigil == "%") && strings.HasPrefix(label, sigil) && len(label) > 1 {
				if replaceRange != nil {
					textEdit = &protocol.TextEdit{Range: *replaceRange, NewText: label}
				} else {
					text := label[1:]
					insertText = &text
				}
			}
		}
		items = append(items, protocol.CompletionItem{
			Label:      label,
			Kind:       &k,
			Detail:     &d,
			InsertText: insertText,
			TextEdit:   textEdit,
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

		if strings.HasPrefix(sym.Name, "@") || strings.HasPrefix(sym.Name, "%") {
			if strings.HasPrefix(prefix, "$") {
				alt := "$" + sym.Name[1:]
				add(alt, protocol.CompletionItemKindVariable, detail+" (sigil)")
			}
		}
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

func methodCompletionItems(methods []string, prefix string, text string, start int, offset int) []protocol.CompletionItem {
	if len(methods) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	items := make([]protocol.CompletionItem, 0, len(methods))
	rng := protocol.Range{
		Start: positionFromOffset(text, start),
		End:   positionFromOffset(text, offset),
	}
	for _, name := range methods {
		if name == "" || !strings.HasPrefix(name, prefix) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		k := protocol.CompletionItemKindMethod
		d := "method"
		items = append(items, protocol.CompletionItem{
			Label:    name,
			Kind:     &k,
			Detail:   &d,
			TextEdit: &protocol.TextEdit{Range: rng, NewText: name},
		})
	}
	return items
}

func methodsForPackage(doc *ppi.Document, pkg string) []string {
	if doc == nil || doc.Root == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := []string{}
	walkNodes(doc.Root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::sub" {
			return
		}
		if n.Name == "" {
			return
		}
		start, _, ok := nodeTokenRange(n)
		if !ok {
			return
		}
		p := doc.PackageAt(start)
		if p != pkg {
			return
		}
		if _, ok := seen[n.Name]; ok {
			return
		}
		seen[n.Name] = struct{}{}
		out = append(out, n.Name)
	})
	return out
}

func methodPrefixForReceivers(text string, offset int, receivers map[string]struct{}) (string, int, string, bool) {
	if len(receivers) == 0 {
		return "", 0, "", false
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	end := offset
	start := end
	for start > 0 {
		ch := text[start-1]
		if isMethodChar(ch) {
			start--
			continue
		}
		break
	}
	prefix := text[start:end]
	i := start
	for i > 0 && (text[i-1] == ' ' || text[i-1] == '\t') {
		i--
	}
	if i < 2 || text[i-2:i] != "->" {
		return "", 0, "", false
	}
	i -= 2
	for i > 0 && (text[i-1] == ' ' || text[i-1] == '\t') {
		i--
	}
	j := i
	for j > 0 && isMethodChar(text[j-1]) {
		j--
	}
	if j == 0 || text[j-1] != '$' {
		return "", 0, "", false
	}
	recv := text[j-1 : i]
	if _, ok := receivers[recv]; !ok {
		return "", 0, "", false
	}
	return prefix, start, recv, true
}

func isMethodChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
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
		diagnostics = append(diagnostics, s.toStrictVarDiagnostics(uri, doc.text, doc.parsed)...)
		diagnostics = append(diagnostics, sigDiagnostics(doc.text)...)
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

func sigDiagnostics(text string) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	if text == "" {
		return nil
	}
	source := "perl-lsp"
	sev := protocol.DiagnosticSeverityError
	offset := 0
	for offset < len(text) {
		lineStart := offset
		lineEnd := len(text)
		if idx := strings.IndexByte(text[offset:], '\n'); idx >= 0 {
			lineEnd = offset + idx
		}
		line := text[lineStart:lineEnd]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			body := strings.TrimSpace(strings.TrimPrefix(trim, "#"))
			if strings.HasPrefix(body, ":SIG") {
				open := strings.IndexByte(body, '(')
				close := strings.LastIndexByte(body, ')')
				if open < 0 || close < open+1 {
					out = append(out, protocol.Diagnostic{
						Range:    protocol.Range{Start: positionFromOffset(text, lineStart), End: positionFromOffset(text, lineEnd)},
						Severity: &sev,
						Source:   &source,
						Message:  "invalid :SIG(...)",
					})
				} else {
					sig := strings.TrimSpace(body[open+1 : close])
					if err := analysis.ValidateSig(sig); err != nil {
						out = append(out, protocol.Diagnostic{
							Range:    protocol.Range{Start: positionFromOffset(text, lineStart), End: positionFromOffset(text, lineEnd)},
							Severity: &sev,
							Source:   &source,
							Message:  "invalid :SIG(...): " + err.Error(),
						})
					}
				}
			}
		}
		if lineEnd >= len(text) {
			break
		}
		offset = lineEnd + 1
	}
	return out
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

func (s *Server) toStrictVarDiagnostics(uri protocol.DocumentUri, text string, doc *ppi.Document) []protocol.Diagnostic {
	var extra map[string]struct{}
	if path, ok := uriToPath(uri); ok {
		extra = s.exportedStrictVars(doc, path)
	}
	diags := analysis.StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) == 0 {
		return nil
	}
	out := make([]protocol.Diagnostic, 0, len(diags))
	source := "perl-lsp"
	sev := protocol.DiagnosticSeverityError
	for _, diag := range diags {
		rng := diagnosticRange(text, diag.Offset)
		out = append(out, protocol.Diagnostic{
			Range:    rng,
			Severity: &sev,
			Source:   &source,
			Message:  diag.Message,
		})
	}
	return out
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

func (s *Server) initWorkspaceIndex(params *protocol.InitializeParams) {
	roots := workspaceRoots(params)
	s.logger.Debug("workspace roots", "roots", roots)
	baseRoots := filterExistingRoots(defaultLibRoots(roots), s.logger)
	incRoots, err := perlINCPaths()
	if err != nil {
		s.logger.Debug("perl @INC lookup failed", "error", err)
	}

	s.workspaceMu.Lock()
	s.workspaceRoots = baseRoots
	s.incRoots = incRoots
	if s.extraRoots == nil {
		s.extraRoots = make(map[string]struct{})
	}
	s.workspaceMu.Unlock()

	merged := uniqueStrings(append(append([]string{}, baseRoots...), incRoots...))
	if len(merged) == 0 {
		s.logger.Debug("workspace index skipped: no roots")
		return
	}
	index, err := analysis.BuildWorkspaceIndex(merged)
	if err != nil {
		s.logger.Debug("workspace index failed", "error", err)
		return
	}
	s.workspaceMu.Lock()
	s.workspaceIndex = index
	s.workspaceMu.Unlock()
	s.logger.Info("workspace index ready", "roots", len(merged), "files", index.Files)
}

func workspaceRoots(params *protocol.InitializeParams) []string {
	if params == nil {
		return nil
	}
	var roots []string
	if len(params.WorkspaceFolders) > 0 {
		for _, folder := range params.WorkspaceFolders {
			if path, ok := uriToPath(folder.URI); ok {
				roots = append(roots, path)
			}
		}
	}
	if len(roots) == 0 && params.RootURI != nil {
		if path, ok := uriToPath(*params.RootURI); ok {
			roots = append(roots, path)
		}
	}
	if len(roots) == 0 && params.RootPath != nil {
		roots = append(roots, *params.RootPath)
	}
	return roots
}

func defaultLibRoots(roots []string) []string {
	var out []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		out = append(out, filepath.Join(root, "lib"))
		out = append(out, filepath.Join(root, "local", "lib", "perl5"))
	}
	return out
}

func (s *Server) findWorkspaceDefinitions(name string, uri protocol.DocumentUri, pkg string, useImports map[string]map[string]struct{}, qualified bool) ([]analysis.Definition, error) {
	s.workspaceMu.RLock()
	index := s.workspaceIndex
	s.workspaceMu.RUnlock()
	if index == nil {
		return nil, nil
	}
	exclude := ""
	if path, ok := uriToPath(uri); ok {
		exclude = path
	}

	if qualified || strings.Contains(name, "::") {
		defs := index.FindPackages(name, exclude)
		if len(defs) > 0 {
			return defs, nil
		}
		return index.FindSubsFull(name, exclude), nil
	}

	if pkg != "" && pkg != "main" {
		full := pkg + "::" + name
		defs := index.FindSubsFull(full, exclude)
		if len(defs) > 0 {
			return defs, nil
		}
	}

	filteredUseImports := filterUseImports(index, useImports, exclude)
	if len(filteredUseImports) == 0 && len(useImports) > 0 {
		s.logger.Debug("use imports filtered", "imports", mapKeys(useImports), "packages", packageCounts(index, useImports, exclude))
	}
	if len(filteredUseImports) == 0 && (pkg == "" || pkg == "main") {
		return nil, nil
	}
	for usePkg, symbols := range filteredUseImports {
		if _, ok := symbols[name]; !ok {
			continue
		}
		full := usePkg + "::" + name
		defs := index.FindSubsFull(full, exclude)
		if len(defs) > 0 {
			return defs, nil
		}
	}
	return nil, nil
}

func (s *Server) moduleLocation(name string, uri protocol.DocumentUri) (protocol.Location, bool) {
	s.workspaceMu.RLock()
	index := s.workspaceIndex
	roots := append([]string{}, s.workspaceRoots...)
	roots = append(roots, s.incRoots...)
	for p := range s.extraRoots {
		roots = append(roots, p)
	}
	s.workspaceMu.RUnlock()
	if index == nil {
		s.logger.Debug("module lookup skipped: no index", "name", name)
		return protocol.Location{}, false
	}
	exclude := ""
	if path, ok := uriToPath(uri); ok {
		exclude = path
	}
	s.logger.Debug("module lookup", "name", name, "exclude", exclude)
	defs := index.FindPackages(name, exclude)
	if len(defs) == 0 {
		s.logger.Debug("module not found in index", "name", name, "roots", uniqueStrings(roots))
		return protocol.Location{}, false
	}
	path := defs[0].File
	if path == "" {
		return protocol.Location{}, false
	}
	rng := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 0, Character: 0},
	}
	return protocol.Location{
		URI:   protocol.DocumentUri(fileURI(path)),
		Range: rng,
	}, true
}

func mapKeys(m map[string]map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func packageCounts(index *analysis.WorkspaceIndex, imports map[string]map[string]struct{}, exclude string) map[string]int {
	if index == nil || len(imports) == 0 {
		return nil
	}
	out := make(map[string]int, len(imports))
	for name := range imports {
		out[name] = len(index.FindPackages(name, exclude))
	}
	return out
}

func (s *Server) ensureUseLibPaths(root *ppi.Node, filePath string) {
	paths := filterExistingRoots(collectUseLibPaths(root, filePath), s.logger)
	if len(paths) == 0 {
		return
	}
	var added bool
	s.workspaceMu.Lock()
	if s.extraRoots == nil {
		s.extraRoots = make(map[string]struct{})
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, ok := s.extraRoots[p]; ok {
			continue
		}
		s.extraRoots[p] = struct{}{}
		added = true
	}
	roots := append([]string{}, s.workspaceRoots...)
	roots = append(roots, s.incRoots...)
	for p := range s.extraRoots {
		roots = append(roots, p)
	}
	s.workspaceMu.Unlock()

	if !added {
		return
	}
	roots = uniqueStrings(roots)
	index, err := analysis.BuildWorkspaceIndex(roots)
	if err != nil {
		s.logger.Debug("workspace index rebuild failed", "error", err)
		return
	}
	s.workspaceMu.Lock()
	s.workspaceIndex = index
	s.workspaceMu.Unlock()
	s.logger.Info("workspace index rebuilt", "roots", len(roots), "files", index.Files)
}

func collectUseLibPaths(root *ppi.Node, filePath string) []string {
	if root == nil {
		return nil
	}
	dir := filepath.Dir(filePath)
	return collectUseLibPathsWithDir(root, dir)
}

func collectUseLibPathsWithBase(root *ppi.Node, filePath, baseDir string) []string {
	if root == nil {
		return nil
	}
	dir := baseDir
	if dir == "" {
		dir = filepath.Dir(filePath)
	}
	return collectUseLibPathsWithDir(root, dir)
}

func collectUseLibPathsWithDir(root *ppi.Node, dir string) []string {
	var out []string
	walkNodes(root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::include" {
			return
		}
		if strings.ToLower(n.Keyword) != "use" || strings.ToLower(n.Name) != "lib" {
			return
		}
		items := n.ImportItems
		if len(items) == 0 {
			items = importItemsFromArgs(n.Args)
		}
		for _, item := range items {
			item = strings.Trim(item, "\"'`")
			if item == "" {
				continue
			}
			if !filepath.IsAbs(item) {
				item = filepath.Join(dir, item)
			}
			out = append(out, item)
		}
	})
	return out
}

func perlINCPaths() ([]string, error) {
	cmd := exec.Command("perl", "-e", "print join(\"\\n\", @INC)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var paths []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, line)
	}
	return paths, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func filterExistingRoots(paths []string, logger *slog.Logger) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			if logger != nil {
				logger.Debug("skip missing root", "path", p, "error", err)
			}
			continue
		}
		out = append(out, p)
	}
	return out
}

func rangeFromFile(path string, start int, end int) (protocol.Range, bool) {
	if start < 0 || end < 0 || end < start {
		return protocol.Range{}, false
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return protocol.Range{}, false
	}
	text := string(src)
	if start > len(text) {
		return protocol.Range{}, false
	}
	if end > len(text) {
		end = len(text)
	}
	return protocol.Range{
		Start: positionFromOffset(text, start),
		End:   positionFromOffset(text, end),
	}, true
}

func uriToPath(uri protocol.DocumentUri) (string, bool) {
	if uri == "" {
		return "", false
	}
	u, err := url.Parse(string(uri))
	if err != nil {
		return "", false
	}
	if u.Scheme != "file" {
		return "", false
	}
	path := u.Path
	if path == "" && u.Opaque != "" {
		path = u.Opaque
	}
	if path == "" {
		return "", false
	}
	path = filepath.FromSlash(path)
	return path, true
}

func fileURI(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return u.String()
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
