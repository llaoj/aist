package server

import (
	"time"

	"github.com/llaoj/aist/internal/buildinfo"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const (
	maxProxyBodyBytes           int64 = 16 << 20
	defaultProxyRequestTimeout        = 85 * time.Second
	defaultUpstreamModelTimeout       = 30 * time.Second
)

// APIOptions configures the server business logic.
type APIOptions struct {
	// ClientDir overrides defaultClientDir in tests; empty uses defaultClientDir.
	ClientDir string
	// Now overrides time.Now in tests; nil uses time.Now.
	Now func() time.Time
	// ProxyRequestTimeout caps total elapsed time for one proxied chat request
	// across all upstream model attempts. Zero uses defaultProxyRequestTimeout (85s).
	ProxyRequestTimeout time.Duration
	// UpstreamModelTimeout caps elapsed time for a single upstream model attempt
	// before failing over to the next model. Zero uses defaultUpstreamModelTimeout (30s).
	UpstreamModelTimeout time.Duration
}

// API wires the business dependencies used by the HTTP handlers.
type API struct {
	clientDir            string
	nowFunc              func() time.Time
	proxyRequestTimeout  time.Duration
	upstreamModelTimeout time.Duration
}

// NewAPI builds the server business logic from opts.
func NewAPI(opts APIOptions) *API {
	clientDir := opts.ClientDir
	if clientDir == "" {
		clientDir = defaultClientDir
	}
	nowFunc := opts.Now
	if nowFunc == nil {
		nowFunc = time.Now
	}
	proxyRequestTimeout := opts.ProxyRequestTimeout
	if proxyRequestTimeout <= 0 {
		proxyRequestTimeout = defaultProxyRequestTimeout
	}
	upstreamModelTimeout := opts.UpstreamModelTimeout
	if upstreamModelTimeout <= 0 {
		upstreamModelTimeout = defaultUpstreamModelTimeout
	}
	return &API{
		clientDir:            clientDir,
		nowFunc:              nowFunc,
		proxyRequestTimeout:  proxyRequestTimeout,
		upstreamModelTimeout: upstreamModelTimeout,
	}
}

func (s *API) now() time.Time {
	return s.nowFunc()
}

// RegisterRoutes registers all custom AIST routes on the given serve event.
func (s *API) RegisterRoutes(router *core.ServeEvent) {
	r := router.Router

	r.GET("/v1/healthz", func(e *core.RequestEvent) error {
		return e.JSON(200, map[string]string{
			"status":  "ok",
			"version": buildinfo.Default.Version,
		})
	})

	r.POST("/v1/register", s.handleRegister).Bind(apis.BodyLimit(maxRegisterBodyBytes))
	r.GET("/v1/account/usage", s.handleAccountUsage).Bind(apis.RequireAuth(usersCollection))
	r.POST("/v1/proxy/chat/completions", s.handleChatCompletions).Bind(
		apis.RequireAuth(usersCollection),
		apis.BodyLimit(maxProxyBodyBytes),
	)

	r.GET("/releases/client/{path...}", s.serveClientRelease)
	registerFrontendRoutes(router)
}
