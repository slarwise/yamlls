package messages

import "go.lsp.dev/protocol"

type InitializeParams struct {
	// Information about the client
	ClientInfo *ClientInfo `json:"clientInfo"`

	// The capabilities provided by the client (editor or tool)
	Capabilities ClientCapabilities `json:"capabilities"`
}

type ClientInfo struct {
	Name    string  `json:"name"`
	Version *string `json:"version"`
}

type ClientCapabilities struct {
}

type InitializeResult struct {
	// The capabilities the language server provides.
	Capabilities protocol.ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo                 `json:"serverInfo"`
}

// https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#serverCapabilities
type ServerCapabilities struct {
	TextDocumentSync   TextDocumentSyncKind `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions   `json:"completionProvider,omitempty"`
	HoverProvider      bool                 `json:"hoverProvider,omitempty"`
}

type TextDocumentSyncKind int

const (
	TextDocumentSyncKindNone TextDocumentSyncKind = iota
	TextDocumentSyncKindFull
	TextDocumentSyncKindIncremental
)

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

type ServerInfo struct {
	Name    string  `json:"name"`
	Version *string `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

const PublishDiagnosticsMethod = "textDocument/publishDiagnostics"

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Diagnostic struct {
	Range    Range               `json:"range"`
	Severity *DiagnosticSeverity `json:"severity"`
	// The diagnostic's code, which might appear in the user interface.
	Code            *string          `json:"code"`
	CodeDescription *CodeDescription `json:"codeDescription"`
	// A human-readable string describing the source of this
	// diagnostic, e.g. 'typescript' or 'super lint'.
	Source             *string                        `json:"source"`
	Message            string                         `json:"message"`
	Tags               []DiagnosticTag                `json:"tags"`
	RelatedInformation []DiagnosticRelatedInformation `json:"relatedInformation"`
	Data               any                            `json:"any"`
}

type CodeDescription struct {
	HREF string `json:"href"`
}

type DiagnosticSeverity int

const (
	DiagnosticSeverityError       DiagnosticSeverity = 1
	DiagnosticSeverityWarning     DiagnosticSeverity = 2
	DiagnosticSeverityInformation DiagnosticSeverity = 3
	DiagnosticSeverityHint        DiagnosticSeverity = 4
)

type DiagnosticTag int

const (
	DiagnosticTagUnnecessary DiagnosticTag = 1
	DiagnosticTagDeprecated  DiagnosticTag = 2
)

type DiagnosticRelatedInformation struct {
	Location Location `json:"location"`
	Message  string   `json:"message"`
}

const DidChangeTextDocumentNotification = "textDocument/didChange"

type DidChangeTextDocumentParams struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`

	/**
	 * The actual content changes. The content changes describe single state
	 * changes to the document. So if there are two content changes c1 (at
	 * array index 0) and c2 (at array index 1) for a document in state S then
	 * c1 moves the document from S to S' and c2 from S' to S''. So c1 is
	 * computed on the state S and c2 is computed on the state S'.
	 *
	 * To mirror the content of a document using change events use the following
	 * approach:
	 * - start with the same initial content
	 * - apply the 'textDocument/didChange' notifications in the order you
	 *   receive them.
	 * - apply the `TextDocumentContentChangeEvent`s in a single notification
	 *   in the order you receive them.
	 */
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type VersionedTextDocumentIdentifier struct {
	URI string `json:"uri"`
	/**
	 * The version number of this document.
	 *
	 * The version number of a document will increase after each change,
	 * including undo/redo. The number doesn't need to be consecutive.
	 */
	Version int `json:"version"`
}

// An event describing a change to a text document. If only a text is provided
// it is considered to be the full content of the document.
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range"`
	Text  string `json:"text"`
}

const DidOpenTextDocumentNotification = "textDocument/didOpen"

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

const ShowMessageMethod = "window/showMessage"

// https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#window_showMessage
type ShowMessageParams struct {
	Type    MessageType `json:"type"`
	Message string      `json:"message"`
}

type MessageType int

const (
	MessageTypeError   MessageType = 1
	MessageTypeWarning MessageType = 2
	MessageTypeInfo    MessageType = 3
	MessageTypeLog     MessageType = 4
)

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

func NewPosition(line, character int) Position {
	return Position{
		Line:      line,
		Character: character,
	}
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type ShutdownParams struct{}
type ShutdownResponse struct {
	Error int `json:"error"`
}
type ExitParams struct{}

type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type HoverResult struct {
	Contents MarkupContent `json:"contents"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CompletionResult []CompletionItem
type CompletionItem struct {
	Label         string        `json:"label"`
	Documentation MarkupContent `json:"documentation"`
}
