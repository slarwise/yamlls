package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/textproto"
	"strconv"
	"sync"
)

const protocolVersion = "2.0"

type Message interface {
	IsJSONRPC() bool
}

type Request struct {
	ProtocolVersion string           `json:"jsonrpc"`
	ID              *json.RawMessage `json:"id"`
	Method          string           `json:"method"`
	Params          json.RawMessage  `json:"params"`
}

func (r Request) IsJSONRPC() bool {
	return r.ProtocolVersion == protocolVersion
}

type OutgoingRequest struct {
	ProtocolVersion string      `json:"jsonrpc"`
	ID              string      `json:"id"`
	Method          string      `json:"method"`
	Params          interface{} `json:"params"`
}

func (r OutgoingRequest) IsJSONRPC() bool {
	return r.ProtocolVersion == protocolVersion
}

func (r Request) IsNotification() bool {
	return r.ID == nil
}

type Response struct {
	ProtocolVersion string           `json:"jsonrpc"`
	ID              *json.RawMessage `json:"id"`
	Result          any              `json:"result"`
	Error           *Error           `json:"error"`
}

func (r Response) IsJSONRPC() bool {
	return r.ProtocolVersion == protocolVersion
}

func NewResponse(id *json.RawMessage, result any) Response {
	return Response{
		ProtocolVersion: protocolVersion,
		ID:              id,
		Result:          result,
		Error:           nil,
	}
}

func NewResponseError(id *json.RawMessage, err error) Response {
	return Response{
		ProtocolVersion: protocolVersion,
		ID:              id,
		Result:          nil,
		Error:           newError(err),
	}
}

type Error struct {
	// Code is a Number that indicates the error type that occurred.
	Code int64 `json:"code"`
	// Message of the error.
	// The message SHOULD be limited to a concise single sentence.
	Message string `json:"message"`
	// A Primitive or Structured value that contains additional information about the error.
	// This may be omitted.
	// The value of this member is defined by the Server (e.g. detailed error information, nested errors etc.).
	Data any `json:"data"`
}

func (e *Error) Error() string {
	return e.Message
}

func newError(err error) *Error {
	return &Error{
		Code:    0,
		Message: err.Error(),
		Data:    nil,
	}
}

var (
	ErrParseError                 *Error = &Error{Code: -32700, Message: "Parse error"}
	ErrInvalidRequest             *Error = &Error{Code: -32600, Message: "Invalid Request"}
	ErrMethodNotFound             *Error = &Error{Code: -32601, Message: "Method not found"}
	ErrInvalidParams              *Error = &Error{Code: -32602, Message: "Invalid params"}
	ErrInternal                   *Error = &Error{Code: -32603, Message: "Internal error"}
	ErrServerNotInitialized       *Error = &Error{Code: -32002, Message: "Server not initialized"}
	ErrInvalidContentLengthHeader        = errors.New("missing or invalid Content-Length header")
)

type Notification struct {
	ProtocolVersion string `json:"jsonrpc"`
	Method          string `json:"method"`
	Params          any    `json:"params"`
}

func (n Notification) IsJSONRPC() bool {
	return n.ProtocolVersion == protocolVersion
}

func Read(r *bufio.Reader) (Request, error) {
	req := Request{}
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		return req, err
	}
	contentLength, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	if err != nil {
		return req, ErrInvalidRequest
	}
	err = json.NewDecoder(io.LimitReader(r, contentLength)).Decode(&req)
	if err != nil {
		return req, nil
	}
	if !req.IsJSONRPC() {
		return req, ErrInvalidRequest
	}
	return req, nil
}

func Write(w *bufio.Writer, msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)))
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	if err != nil {
		return err
	}
	err = w.Flush()
	return err
}

func NewMux(log *slog.Logger, r io.Reader, w io.Writer) *Mux {
	return &Mux{
		reader:               bufio.NewReader(r),
		concurrencyLimit:     4,
		methodHandlers:       map[string]MethodHandler{},
		notificationHandlers: map[string]NotificationHandler{},
		writer:               bufio.NewWriter(w),
		writeLock:            &sync.Mutex{},
		log:                  log,
		error: func(err error) {
			return
		},
	}
}

type Mux struct {
	initialized          bool
	reader               *bufio.Reader
	concurrencyLimit     int64
	methodHandlers       map[string]MethodHandler
	notificationHandlers map[string]NotificationHandler
	writer               *bufio.Writer
	writeLock            *sync.Mutex
	log                  *slog.Logger
	error                func(err error)
}

type MethodHandler func(params json.RawMessage) (any, error)
type NotificationHandler func(params json.RawMessage) error

func (m *Mux) HandleMethod(name string, method MethodHandler) {
	m.methodHandlers[name] = method
}

func (m *Mux) HandleNotification(name string, notification NotificationHandler) {
	m.notificationHandlers[name] = notification
}

func (m *Mux) Notify(method string, params any) error {
	n := Notification{
		ProtocolVersion: protocolVersion,
		Method:          method,
		Params:          params,
	}
	return m.write(n)
}

func (m *Mux) Request(method string, params any) error {
	r := OutgoingRequest{
		ProtocolVersion: protocolVersion,
		ID:              "1",
		Method:          method,
		Params:          params,
	}
	return m.write(r)
}

func ptr[T any](v T) *T {
	return &v
}

func (m *Mux) write(msg Message) error {
	m.writeLock.Lock()
	defer m.writeLock.Unlock()
	return Write(m.writer, msg)
}

func (m *Mux) Process() error {
	for {
		req, err := Read(m.reader)
		if err != nil {
			return err
		}
		if req.IsNotification() {
			if req.Method != "exit" {
				m.log.Warn("Dropping notification sent before initialization", slog.Any("req", req))
				continue
			}
			m.handleMessage(req)
			continue
		}
		if req.Method != "initialize" {
			m.log.Warn("The client sent a method before initialization", slog.Any("req", req))
			if err = m.write(NewResponseError(req.ID, ErrServerNotInitialized)); err != nil {
				return err
			}
			continue
		}
		m.handleMessage(req)
		break
	}
	m.log.Info("Initialization complete")

	sem := make(chan struct{}, m.concurrencyLimit)
	for {
		sem <- struct{}{}
		req, err := Read(m.reader)
		if err != nil {
			return err
		}
		go func(req Request) {
			m.handleMessage(req)
			<-sem
		}(req)
	}
}

func (m *Mux) handleMessage(req Request) {
	if req.IsNotification() {
		m.handleNotification(req)
		return
	}
	m.handleRequestResponse(req)
}

func (m *Mux) handleNotification(req Request) {
	log := m.log.With(slog.String("method", req.Method))
	nh, ok := m.notificationHandlers[req.Method]
	if !ok {
		log.Warn("No notification handler found")
		return
	}
	if err := nh(req.Params); err != nil && m.error != nil {
		log.Error("Failed to handle notification", slog.Any("error", err))
		m.error(err)
	}
}

func (m *Mux) handleRequestResponse(req Request) {
	log := m.log.With(slog.Any("id", req.ID), slog.String("method", req.Method))
	mh, ok := m.methodHandlers[req.Method]
	if !ok {
		log.Error("No method handler found")
		if err := m.write(NewResponseError(req.ID, ErrMethodNotFound)); err != nil {
			log.Error("Failed to respond", slog.Any("error", err))
			m.error(fmt.Errorf("Failed to respond: %w", err))
		}
		return
	}
	var res Response
	result, err := mh(req.Params)
	if err != nil {
		log.Error("Failed to handle", slog.Any("error", err))
		res = NewResponseError(req.ID, err)
	} else {
		res = NewResponse(req.ID, result)
	}
	if err = m.write(res); err != nil {
		log.Error("Failed to respond", slog.Any("error", err))
		m.error(fmt.Errorf("Failed to response: %w", err))
	}
}
