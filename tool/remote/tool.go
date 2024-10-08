package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Songmu/flextime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mashiike/bedrocktool"
)

type SpecificationCache struct {
	mu             sync.RWMutex
	cache          map[string]Specification
	cacheAt        map[string]time.Time
	expireDuration time.Duration
}

func NewSpecificationCache(expireDuration time.Duration) *SpecificationCache {
	return &SpecificationCache{
		cache:          make(map[string]Specification),
		cacheAt:        make(map[string]time.Time),
		expireDuration: expireDuration,
	}
}

func (sc *SpecificationCache) Get(name string) (Specification, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	spec, ok := sc.cache[name]
	if !ok {
		return Specification{}, false
	}
	at, ok := sc.cacheAt[name]
	if !ok {
		return Specification{}, false
	}
	if flextime.Since(at) > sc.expireDuration {
		sc.mu.RUnlock()
		sc.Delete(name)
		sc.mu.RLock()
		return Specification{}, false
	}
	return spec, ok
}

func (sc *SpecificationCache) Set(name string, spec Specification) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cache[name] = spec
	sc.cacheAt[name] = flextime.Now()
}

func (sc *SpecificationCache) Delete(name string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.cache, name)
	delete(sc.cacheAt, name)
}

var DefaultSpecificationCache = NewSpecificationCache(15 * time.Minute)

type Tool struct {
	endpoint      *url.URL
	baseEndpoint  *url.URL
	spec          Specification
	newReqFunc    RequestConstructor
	inputSchema   document.Interface
	client        *http.Client
	newErr        func(error) (types.ToolResultBlock, error)
	signer        func(*http.Request, string) (*http.Request, error)
	respValidator func(*http.Response, *http.Request) error
}

type RequestConstructor func(ctx context.Context, method string, url string, toolUse types.ToolUseBlock) (*http.Request, error)

var DefaultRequestConstructor = func(ctx context.Context, method string, url string, toolUse types.ToolUseBlock) (*http.Request, error) {
	bs, err := toolUse.Input.MarshalSmithyDocument()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if toolUse.ToolUseId != nil {
		req.Header.Set(HeaderToolUseID, *toolUse.ToolUseId)
	}
	if toolUse.Name != nil {
		req.Header.Set(HeaderToolName, *toolUse.Name)
	}
	return req, nil
}

type ToolConfig struct {
	Endpoint           string
	SpecificationPath  string
	SpecificationCache *SpecificationCache
	RequestConstructor RequestConstructor
	HTTPClient         *http.Client
	ErrorConstractor   func(error) (types.ToolResultBlock, error)
	RequestSigner      func(req *http.Request, subject string) (*http.Request, error)
	ResponseValidator  func(resp *http.Response, req *http.Request) error
}

type remoteWorker struct {
	tool *Tool
}

func NewTool(ctx context.Context, cfg ToolConfig) (*Tool, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	if cfg.SpecificationCache == nil {
		cfg.SpecificationCache = DefaultSpecificationCache
	}
	if cfg.RequestConstructor == nil {
		cfg.RequestConstructor = DefaultRequestConstructor
	}
	if cfg.SpecificationPath == "" {
		cfg.SpecificationPath = DefaultSpecificationPath
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.ErrorConstractor == nil {
		cfg.ErrorConstractor = func(err error) (types.ToolResultBlock, error) {
			return types.ToolResultBlock{}, err
		}
	}
	if cfg.RequestSigner == nil {
		cfg.RequestSigner = func(req *http.Request, _ string) (*http.Request, error) {
			return req, nil
		}
	}
	if cfg.ResponseValidator == nil {
		cfg.ResponseValidator = func(_ *http.Response, _ *http.Request) error {
			return nil
		}
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	t := &Tool{
		endpoint:      u.JoinPath(cfg.SpecificationPath),
		baseEndpoint:  u,
		newReqFunc:    cfg.RequestConstructor,
		client:        cfg.HTTPClient,
		newErr:        cfg.ErrorConstractor,
		signer:        cfg.RequestSigner,
		respValidator: cfg.ResponseValidator,
	}
	spec, ok := cfg.SpecificationCache.Get(u.String())
	if !ok {
		spec, err = t.fetchSpecification(ctx)
		if err != nil {
			return nil, err
		}
		cfg.SpecificationCache.Set(u.String(), spec)
	}
	t.spec = spec
	if bedrocktool.ValidateToolName(spec.Name) != nil {
		return nil, errors.New("invalid tool name")
	}
	var v interface{}
	if err := json.Unmarshal(spec.InputSchema, &v); err != nil {
		return nil, err
	}
	t.inputSchema = document.NewLazyDocument(v)
	return t, nil
}

func (t *Tool) SignRequest(req *http.Request, subject string) (*http.Request, error) {
	return t.signer(req, subject)
}

func (t *Tool) fetchSpecification(ctx context.Context) (Specification, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.endpoint.String(), nil)
	if err != nil {
		return Specification{}, err
	}
	req, err = t.signer(req, "specification")
	if err != nil {
		return Specification{}, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return Specification{}, err
	}
	defer resp.Body.Close()
	if err := t.respValidator(resp, req); err != nil {
		return Specification{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Specification{}, errors.New("failed to fetch specification")
	}
	var spec Specification
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return Specification{}, err
	}
	workerEndpoint, err := url.Parse(spec.WorkerEndpoint)
	if err != nil {
		return Specification{}, fmt.Errorf("failed to parse worker endpoint; %w", err)
	}
	if !workerEndpoint.IsAbs() {
		workerEndpoint = t.baseEndpoint.ResolveReference(workerEndpoint)
	}
	spec.WorkerEndpoint = workerEndpoint.String()
	return spec, nil
}

var _ bedrocktool.Tool = (*Tool)(nil)
var _ bedrocktool.Worker = (*remoteWorker)(nil)

func (t *Tool) Name() string {
	return t.spec.Name
}

func (t *Tool) Description() string {
	return t.spec.Description
}

func (t *Tool) Specification() Specification {
	return t.spec
}

func (t *Tool) Worker() bedrocktool.Worker {
	return &remoteWorker{
		tool: t,
	}
}

func (w *remoteWorker) InputSchema() document.Interface {
	return w.tool.inputSchema
}

func (w *remoteWorker) Execute(ctx context.Context, toolUse types.ToolUseBlock) (types.ToolResultBlock, error) {
	req, err := w.tool.newReqFunc(ctx, http.MethodPost, w.tool.spec.WorkerEndpoint, toolUse)
	if err != nil {
		return w.tool.newErr(fmt.Errorf("failed to create request; %w", err))
	}
	subject := "tool/" + w.tool.Name()
	if toolUse.ToolUseId != nil {
		subject += "/" + *toolUse.ToolUseId
	}
	req, err = w.tool.signer(req, subject)
	if err != nil {
		return w.tool.newErr(fmt.Errorf("failed to sign request; %w", err))
	}
	resp, err := w.tool.client.Do(req)
	if err != nil {
		return w.tool.newErr(fmt.Errorf("failed to fetch url; %w", err))
	}
	if err := w.tool.respValidator(resp, req); err != nil {
		return w.tool.newErr(fmt.Errorf("failed to validate response; %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return w.tool.newErr(errors.New("status code is not 200"))
	}
	var tr ToolResult
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return w.tool.newErr(fmt.Errorf("failed to decode response; %w", err))
	}
	trb, err := tr.MarshalTypes()
	if err != nil {
		return w.tool.newErr(fmt.Errorf("failed to marshal response; %w", err))
	}
	return trb, nil
}
