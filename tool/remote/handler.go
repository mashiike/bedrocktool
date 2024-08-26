package remote

import (
	"bytes"
	"encoding/json"
	"errors"
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
	Endpoint          *url.URL
	WorkerPath        string
	ToolName          string
	ToolDescription   string
	SpecificationPath string
	Worker            bedrocktool.Worker
	Logger            *slog.Logger
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
	if cfg.Endpoint == nil {
		return nil, errors.New("endpoint is required")
	}
	h.workerEndpoint = cfg.Endpoint.JoinPath(cfg.WorkerPath)
	if cfg.SpecificationPath == "" {
		cfg.SpecificationPath = DefaultSpecificationPath
	}
	h.specificationEndpoint = cfg.Endpoint.JoinPath(cfg.SpecificationPath)
	if cfg.Worker == nil {
		return nil, errors.New("worker is required")
	}
	var err error
	h.inputSchema, err = cfg.Worker.InputSchema().MarshalSmithyDocument()
	if err != nil {
		return nil, err
	}
	if h.cfg.Logger == nil {
		h.cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	}
	h.mux.HandleFunc("/"+strings.TrimPrefix(h.workerEndpoint.Path, "/"), h.serveHTTPWorker)
	h.mux.HandleFunc("/"+strings.TrimPrefix(h.specificationEndpoint.Path, "/"), h.serveHTTPSpecification)
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.cfg.Logger.InfoContext(r.Context(), "request", "method", r.Method, "url", r.URL)
	h.mux.ServeHTTP(w, r)
}

var (
	HeaderToolUseID = "Bedrock-Tool-Use-Id"
	HeaderToolName  = "Bedrock-Tool-Name"
)

func (h *Handler) serveHTTPWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var v interface{}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		h.cfg.Logger.WarnContext(ctx, "failed to decode request body", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	toolUse := types.ToolUseBlock{
		Input:     document.NewLazyDocument(v),
		Name:      aws.String(h.cfg.ToolName),
		ToolUseId: aws.String(r.Header.Get(HeaderToolUseID)),
	}
	result, err := h.cfg.Worker.Execute(r.Context(), toolUse)
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
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(tr); err != nil {
		h.cfg.Logger.WarnContext(ctx, "failed to encode response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

func (h *Handler) serveHTTPSpecification(w http.ResponseWriter, _ *http.Request) {
	spec := Specification{
		Name:           h.cfg.ToolName,
		Description:    h.cfg.ToolDescription,
		InputSchema:    h.inputSchema,
		WorkerEndpoint: h.workerEndpoint.String(),
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(spec); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}
