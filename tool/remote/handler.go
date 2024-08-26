package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mashiike/bedrocktool"
)

type Handler struct {
	cfg                   HandlerConfig
	workerEndpoint        *url.URL
	specificationEndpoint *url.URL
	inputSchema           json.RawMessage
	mux                   *http.ServeMux
}

type HandlerConfig struct {
	Endpoint                *url.URL
	WorkerPath              string
	ToolName                string
	ToolDescription         string
	SpecificationPath       string
	ErrorHandler            func(w http.ResponseWriter, r *http.Request, err error, code int)
	MethodNotAllowedHandler func(w http.ResponseWriter, r *http.Request)
	NotFoundHandler         func(w http.ResponseWriter, r *http.Request)
	Worker                  bedrocktool.Worker
	Logger                  *slog.Logger
}

var _ http.Handler = (*Handler)(nil)

func NewHandler(cfg HandlerConfig) (*Handler, error) {
	h := &Handler{
		cfg: cfg,
		mux: http.NewServeMux(),
	}
	if bedrocktool.ValidateToolName(cfg.ToolName) != nil {
		return nil, errors.New("invalid tool name")
	}
	if cfg.Worker == nil {
		return nil, errors.New("worker is required")
	}
	if cfg.Endpoint == nil {
		cfg.Endpoint = &url.URL{
			Scheme: "https",
		}
	}
	h.workerEndpoint = cfg.Endpoint.JoinPath(cfg.WorkerPath)
	if cfg.SpecificationPath == "" {
		cfg.SpecificationPath = DefaultSpecificationPath
	}
	h.specificationEndpoint = cfg.Endpoint.JoinPath(cfg.SpecificationPath)
	if cfg.Worker == nil {
		return nil, errors.New("worker is required")
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error, code int) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(code)
			var body bytes.Buffer
			if err := json.NewEncoder(&body).Encode(map[string]any{
				"error":   http.StatusText(code),
				"message": err.Error(),
				"status":  code,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	if cfg.NotFoundHandler == nil {
		cfg.NotFoundHandler = func(w http.ResponseWriter, r *http.Request) {
			cfg.ErrorHandler(w, r, fmt.Errorf("the requested resource %q was not found", r.URL.Path), http.StatusNotFound)
		}
	}
	if cfg.MethodNotAllowedHandler == nil {
		cfg.MethodNotAllowedHandler = func(w http.ResponseWriter, r *http.Request) {
			cfg.ErrorHandler(w, r, fmt.Errorf("the requested resource %q does not support the method %q", r.URL.Path, r.Method), http.StatusMethodNotAllowed)
		}
	}
	var err error
	h.inputSchema, err = cfg.Worker.InputSchema().MarshalSmithyDocument()
	if err != nil {
		return nil, err
	}
	if h.cfg.Logger == nil {
		h.cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	}
	h.mux.HandleFunc("/"+strings.TrimPrefix(h.workerEndpoint.Path, "/"),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				h.cfg.MethodNotAllowedHandler(w, r)
				return
			}
			h.serveHTTPWorker(w, r)
		},
	)
	h.mux.HandleFunc("/"+strings.TrimPrefix(h.specificationEndpoint.Path, "/"),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				h.cfg.MethodNotAllowedHandler(w, r)
				return
			}
			h.serveHTTPSpecification(w, r)
		},
	)
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.cfg.Logger.InfoContext(r.Context(), "request", "method", r.Method, "url", r.URL)
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	matched, pattern := h.mux.Handler(r)
	if pattern == "" {
		h.cfg.NotFoundHandler(w, r)
		return
	}
	matched.ServeHTTP(w, r)
}

var (
	HeaderToolUseID = "Bedrock-Tool-Use-Id"
	HeaderToolName  = "Bedrock-Tool-Name"
)

// WorkerHandler returns an http.Handler that serves the worker endpoint.
func (h *Handler) WorkerHandler() http.Handler {
	return http.HandlerFunc(h.serveHTTPWorker)
}

func (h *Handler) serveHTTPWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var v interface{}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		h.cfg.Logger.WarnContext(ctx, "failed to decode request body", "error", err)
		h.cfg.ErrorHandler(w, r, fmt.Errorf("failed to decode request body: %w", err), http.StatusBadRequest)
		return
	}
	toolUse := types.ToolUseBlock{
		Input:     document.NewLazyDocument(v),
		Name:      aws.String(h.cfg.ToolName),
		ToolUseId: aws.String(r.Header.Get(HeaderToolUseID)),
	}
	result, err := h.cfg.Worker.Execute(ctx, toolUse)
	var tr ToolResult
	if err != nil {
		tr = ToolResult{
			Status: "error",
			Content: []ToolResultContent{
				{
					Type: "text",
					Text: err.Error(),
				},
			},
		}
	} else {
		if err := tr.UnmarshalTypes(result); err != nil {
			h.cfg.Logger.WarnContext(ctx, "failed to marshal tool result", "error", err)
			h.cfg.ErrorHandler(w, r, fmt.Errorf("failed to marshal tool result: %w", err), http.StatusInternalServerError)
			return
		}
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(tr); err != nil {
		h.cfg.Logger.WarnContext(ctx, "failed to encode response", "error", err)
		h.cfg.ErrorHandler(w, r, fmt.Errorf("failed to encode response: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// SpecificationHandler returns an http.Handler that serves the tool specification.
func (h *Handler) SpecificationHandler() http.Handler {
	return http.HandlerFunc(h.serveHTTPSpecification)
}

func (h *Handler) serveHTTPSpecification(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	workerEndpoint := *h.workerEndpoint
	if workerEndpoint.Host == "" {
		workerEndpoint.Host = req.Host
		workerEndpoint.Scheme = req.URL.Scheme
	}
	spec := Specification{
		Name:           h.cfg.ToolName,
		Description:    h.cfg.ToolDescription,
		InputSchema:    h.inputSchema,
		WorkerEndpoint: workerEndpoint.String(),
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(spec); err != nil {
		h.cfg.Logger.WarnContext(ctx, "failed to encode specification", "error", err)
		h.cfg.ErrorHandler(w, req, fmt.Errorf("failed to encode specification: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}
