package router

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
)

const exportSpecVersion = "1.0"

// ExportSpec describes the exported router metadata document.
type ExportSpec struct {
	Version    string          `json:"version" yaml:"version"`
	ExportedAt string          `json:"exportedAt" yaml:"exportedAt"`
	Service    ServiceMetadata `json:"service" yaml:"service"`
	Routes     []RouteMetadata `json:"routes" yaml:"routes"`
}

// ServiceMetadata captures service-wide router configuration.
type ServiceMetadata struct {
	Name            string             `json:"name" yaml:"name"`
	GlobalTimeout   string             `json:"globalTimeout,omitempty" yaml:"globalTimeout,omitempty"`
	GlobalMaxBody   int64              `json:"globalMaxBodySize,omitempty" yaml:"globalMaxBodySize,omitempty"`
	GlobalRateLimit *RateLimitMetadata `json:"globalRateLimit,omitempty" yaml:"globalRateLimit,omitempty"`
	CORS            *CORSMetadata      `json:"cors,omitempty" yaml:"cors,omitempty"`
	TraceIDEnabled  bool               `json:"traceIdEnabled" yaml:"traceIdEnabled"`
}

// CORSMetadata captures CORS settings.
type CORSMetadata struct {
	Origins          []string `json:"origins" yaml:"origins"`
	Methods          []string `json:"methods" yaml:"methods"`
	Headers          []string `json:"headers" yaml:"headers"`
	ExposeHeaders    []string `json:"exposeHeaders,omitempty" yaml:"exposeHeaders,omitempty"`
	AllowCredentials bool     `json:"allowCredentials" yaml:"allowCredentials"`
	MaxAge           string   `json:"maxAge,omitempty" yaml:"maxAge,omitempty"`
}

// RouteMetadata describes a registered route.
type RouteMetadata struct {
	Path           string             `json:"path" yaml:"path"`
	Methods        []string           `json:"methods" yaml:"methods"`
	AuthLevel      string             `json:"authLevel" yaml:"authLevel"`
	Request        *RequestSchema     `json:"request,omitempty" yaml:"request,omitempty"`
	Response       *TypeSchema        `json:"response,omitempty" yaml:"response,omitempty"`
	Timeout        string             `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	MaxBodySize    int64              `json:"maxBodySize,omitempty" yaml:"maxBodySize,omitempty"`
	RateLimit      *RateLimitMetadata `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`
	AuthToken      *AuthTokenMetadata `json:"authToken,omitempty" yaml:"authToken,omitempty"`
	SubRouter      string             `json:"subRouter,omitempty" yaml:"subRouter,omitempty"`
	DisableTimeout bool               `json:"disableTimeout,omitempty" yaml:"disableTimeout,omitempty"`
}

// RequestSchema describes generic route request extraction and shape.
type RequestSchema struct {
	Source       string      `json:"source" yaml:"source"`
	SourceKey    string      `json:"sourceKey,omitempty" yaml:"sourceKey,omitempty"`
	Codec        string      `json:"codec" yaml:"codec"`
	Schema       *TypeSchema `json:"schema" yaml:"schema"`
	HasSanitizer bool        `json:"hasSanitizer,omitempty" yaml:"hasSanitizer,omitempty"`
}

// TypeSchema describes a Go type tree.
type TypeSchema struct {
	TypeName string        `json:"typeName" yaml:"typeName"`
	Package  string        `json:"package,omitempty" yaml:"package,omitempty"`
	Kind     string        `json:"kind" yaml:"kind"`
	Fields   []FieldSchema `json:"fields,omitempty" yaml:"fields,omitempty"`
}

// FieldSchema describes a reflected struct field.
type FieldSchema struct {
	Name     string      `json:"name" yaml:"name"`
	JSONName string      `json:"jsonName,omitempty" yaml:"jsonName,omitempty"`
	Type     string      `json:"type" yaml:"type"`
	Required bool        `json:"required,omitempty" yaml:"required,omitempty"`
	Schema   *TypeSchema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// RateLimitMetadata describes exported rate limiting config.
type RateLimitMetadata struct {
	BucketName string `json:"bucketName,omitempty" yaml:"bucketName,omitempty"`
	Limit      int    `json:"limit" yaml:"limit"`
	Window     string `json:"window" yaml:"window"`
	Strategy   string `json:"strategy" yaml:"strategy"`
}

// AuthTokenMetadata describes auth token extraction settings.
type AuthTokenMetadata struct {
	Source     string `json:"source" yaml:"source"`
	HeaderName string `json:"headerName,omitempty" yaml:"headerName,omitempty"`
	CookieName string `json:"cookieName,omitempty" yaml:"cookieName,omitempty"`
}

// ExportSpec returns a snapshot export document for the router.
func (r *Router[T, U]) ExportSpec() *ExportSpec {
	service := ServiceMetadata{
		Name:            r.config.ServiceName,
		GlobalTimeout:   durationString(r.config.GlobalTimeout),
		GlobalMaxBody:   r.config.GlobalMaxBodySize,
		GlobalRateLimit: rateLimitMetadataFromConfig(r.config.GlobalRateLimit),
		TraceIDEnabled:  r.config.TraceIDBufferSize > 0,
	}
	if r.config.CORSConfig != nil {
		service.CORS = &CORSMetadata{
			Origins:          append([]string(nil), r.config.CORSConfig.Origins...),
			Methods:          append([]string(nil), r.config.CORSConfig.Methods...),
			Headers:          append([]string(nil), r.config.CORSConfig.Headers...),
			ExposeHeaders:    append([]string(nil), r.config.CORSConfig.ExposeHeaders...),
			AllowCredentials: r.config.CORSConfig.AllowCredentials,
			MaxAge:           durationString(r.config.CORSConfig.MaxAge),
		}
	}

	r.metadataMu.RLock()
	routes := append([]RouteMetadata(nil), r.routeMetadata...)
	r.metadataMu.RUnlock()

	return &ExportSpec{
		Version:    exportSpecVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Service:    service,
		Routes:     routes,
	}
}

// ExportJSON writes the spec as indented JSON.
func (r *Router[T, U]) ExportJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.ExportSpec())
}

// ExportJSONFile writes the spec to a file path as indented JSON.
func (r *Router[T, U]) ExportJSONFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return r.ExportJSON(f)
}

func (r *Router[T, U]) appendRouteMetadata(metadata RouteMetadata) {
	r.metadataMu.Lock()
	r.routeMetadata = append(r.routeMetadata, metadata)
	r.metadataMu.Unlock()
}

func durationString(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

func authLevelString(level *AuthLevel) string {
	if level == nil {
		return "none"
	}
	switch *level {
	case AuthRequired:
		return "required"
	case AuthOptional:
		return "optional"
	default:
		return "none"
	}
}

func sourceTypeString(source SourceType) string {
	switch source {
	case Body:
		return "body"
	case Base64QueryParameter:
		return "base64_query"
	case Base62QueryParameter:
		return "base62_query"
	case Base64PathParameter:
		return "base64_path"
	case Base62PathParameter:
		return "base62_path"
	case Empty:
		return "empty"
	default:
		return "unknown"
	}
}

func rateLimitStrategyString(strategy common.RateLimitStrategy) string {
	switch strategy {
	case common.StrategyUser:
		return "user"
	case common.StrategyCustom:
		return "custom"
	default:
		return "ip"
	}
}

func rateLimitMetadataFromConfig(config *common.RateLimitConfig[any, any]) *RateLimitMetadata {
	if config == nil {
		return nil
	}
	return &RateLimitMetadata{
		BucketName: config.BucketName,
		Limit:      config.Limit,
		Window:     durationString(config.Window),
		Strategy:   rateLimitStrategyString(config.Strategy),
	}
}

func rateLimitMetadataFromRuntimeConfig[T comparable, U any](config *common.RateLimitConfig[T, U]) *RateLimitMetadata {
	if config == nil {
		return nil
	}
	return &RateLimitMetadata{
		BucketName: config.BucketName,
		Limit:      config.Limit,
		Window:     durationString(config.Window),
		Strategy:   rateLimitStrategyString(config.Strategy),
	}
}

func authTokenMetadataFromConfig(config common.AuthTokenConfig) *AuthTokenMetadata {
	source := "header"
	if config.Source == common.AuthTokenSourceCookie {
		source = "cookie"
	}
	return &AuthTokenMetadata{
		Source:     source,
		HeaderName: config.HeaderName,
		CookieName: config.CookieName,
	}
}
