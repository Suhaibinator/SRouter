# SRouter Route Export Specification

**Status:** Draft
**Date:** 2025-03-25
**Scope:** Phase 1 — Route Export

---

## Overview

SRouter will gain the ability to export a complete, structured description of all registered routes to a file (JSON or YAML). This export captures everything a client application needs to understand the API surface: endpoints, methods, authentication requirements, request/response schemas, rate limits, and more.

A Phase 2 companion application (out of scope for this spec) will import this file and provide a Postman-like GUI for constructing and sending requests against the described API.

---

## Motivation

Today, SRouter registers routes programmatically but offers no machine-readable description of the resulting API. Developers who want to explore or test an SRouter-powered service must read source code or maintain separate documentation. An export capability solves this by:

1. Providing a single source of truth for the API surface, derived directly from the running router configuration.
2. Enabling tooling (Phase 2 client, documentation generators, SDK generators) to consume the API definition without access to source code.
3. Reducing drift between implementation and documentation — the export is always accurate because it comes from the actual router.

---

## Phase 1: Route Export

### 1. Capture Route Metadata at Registration Time

**Problem:** Currently, route configuration objects (`RouteConfig`, `RouteConfigBase`, `GenericRouteDefinition`) are consumed during registration and discarded. The underlying `httprouter.Router` does not expose registered routes for introspection.

**Solution:** During route registration, collect and store a metadata snapshot for each route in a new field on the `Router` struct. This metadata is a plain data struct (no function references) suitable for serialization.

#### Route Metadata Struct

```go
// RouteMetadata describes a single registered route for export.
type RouteMetadata struct {
    // Path is the full route path (with prefix applied), e.g. "/api/v1/users/:id"
    Path    string   `json:"path" yaml:"path"`
    Methods []string `json:"methods" yaml:"methods"`

    // Authentication
    AuthLevel string `json:"authLevel" yaml:"authLevel"` // "none", "optional", "required"

    // Request schema (generic routes only; nil for standard routes)
    Request *RequestSchema `json:"request,omitempty" yaml:"request,omitempty"`

    // Response schema (generic routes only; nil for standard routes)
    Response *TypeSchema `json:"response,omitempty" yaml:"response,omitempty"`

    // Limits and timeouts
    Timeout     string `json:"timeout,omitempty" yaml:"timeout,omitempty"`         // e.g. "30s"
    MaxBodySize int64  `json:"maxBodySize,omitempty" yaml:"maxBodySize,omitempty"` // bytes

    // Rate limiting
    RateLimit *RateLimitMetadata `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`

    // Auth token configuration
    AuthToken *AuthTokenMetadata `json:"authToken,omitempty" yaml:"authToken,omitempty"`

    // Grouping
    SubRouter string `json:"subRouter,omitempty" yaml:"subRouter,omitempty"` // sub-router path prefix

    // Whether timeout is explicitly disabled
    DisableTimeout bool `json:"disableTimeout,omitempty" yaml:"disableTimeout,omitempty"`
}
```

#### Request Schema

For generic routes, the request schema describes how the request data is sourced and what shape it takes:

```go
type RequestSchema struct {
    // Source indicates where request data comes from
    Source    string `json:"source" yaml:"source"`             // "body", "base64_query", "base62_query", "base64_path", "base62_path", "empty"
    SourceKey string `json:"sourceKey,omitempty" yaml:"sourceKey,omitempty"` // query/path parameter name

    // Codec describes the serialization format
    Codec string `json:"codec" yaml:"codec"` // "json", "proto"

    // Schema describes the request type structure
    Schema *TypeSchema `json:"schema" yaml:"schema"`

    // HasSanitizer indicates a validation/transform step exists
    HasSanitizer bool `json:"hasSanitizer,omitempty" yaml:"hasSanitizer,omitempty"`
}
```

#### Type Schema (Reflection-Based)

Type schemas are derived via Go reflection at registration time to describe the structure of request and response types:

```go
type TypeSchema struct {
    TypeName string             `json:"typeName" yaml:"typeName"`                       // Go type name, e.g. "CreateUserRequest"
    Package  string             `json:"package,omitempty" yaml:"package,omitempty"`      // Go package path
    Kind     string             `json:"kind" yaml:"kind"`                               // "struct", "string", "int", "slice", "map", etc.
    Fields   []FieldSchema      `json:"fields,omitempty" yaml:"fields,omitempty"`        // struct fields
}

type FieldSchema struct {
    Name     string      `json:"name" yaml:"name"`                           // Go field name
    JSONName string      `json:"jsonName,omitempty" yaml:"jsonName,omitempty"` // from `json` struct tag
    Type     string      `json:"type" yaml:"type"`                           // type description, e.g. "string", "int64", "[]Item"
    Required bool        `json:"required,omitempty" yaml:"required,omitempty"` // true if no omitempty tag
    Schema   *TypeSchema `json:"schema,omitempty" yaml:"schema,omitempty"`   // nested struct schema (recursive)
}
```

For JSON-encoded routes, field names and `omitempty` are derived from the `json` struct tag. For Protobuf routes, field names come from `protobuf` / `json` tags on the generated Go types.

#### Rate Limit Metadata

```go
type RateLimitMetadata struct {
    BucketName string `json:"bucketName,omitempty" yaml:"bucketName,omitempty"`
    Limit      int    `json:"limit" yaml:"limit"`
    Window     string `json:"window" yaml:"window"`       // e.g. "1m"
    Strategy   string `json:"strategy" yaml:"strategy"`   // "ip", "user", "custom"
}
```

#### Auth Token Metadata

```go
type AuthTokenMetadata struct {
    Source     string `json:"source" yaml:"source"`                             // "header" or "cookie"
    HeaderName string `json:"headerName,omitempty" yaml:"headerName,omitempty"` // e.g. "Authorization"
    CookieName string `json:"cookieName,omitempty" yaml:"cookieName,omitempty"`
}
```

### 2. Top-Level Export Schema

The full export document wraps route metadata with service-level configuration:

```go
type ExportSpec struct {
    // Spec metadata
    Version   string `json:"version" yaml:"version"`     // spec format version, e.g. "1.0"
    ExportedAt string `json:"exportedAt" yaml:"exportedAt"` // RFC3339 timestamp

    // Service-level info
    Service ServiceMetadata `json:"service" yaml:"service"`

    // All registered routes
    Routes []RouteMetadata `json:"routes" yaml:"routes"`
}

type ServiceMetadata struct {
    Name           string              `json:"name" yaml:"name"`
    GlobalTimeout  string              `json:"globalTimeout,omitempty" yaml:"globalTimeout,omitempty"`
    GlobalMaxBody  int64               `json:"globalMaxBodySize,omitempty" yaml:"globalMaxBodySize,omitempty"`
    GlobalRateLimit *RateLimitMetadata `json:"globalRateLimit,omitempty" yaml:"globalRateLimit,omitempty"`
    CORS           *CORSMetadata       `json:"cors,omitempty" yaml:"cors,omitempty"`
    TraceIDEnabled bool                `json:"traceIdEnabled" yaml:"traceIdEnabled"`
}

type CORSMetadata struct {
    Origins          []string `json:"origins" yaml:"origins"`
    Methods          []string `json:"methods" yaml:"methods"`
    Headers          []string `json:"headers" yaml:"headers"`
    ExposeHeaders    []string `json:"exposeHeaders,omitempty" yaml:"exposeHeaders,omitempty"`
    AllowCredentials bool     `json:"allowCredentials" yaml:"allowCredentials"`
    MaxAge           string   `json:"maxAge,omitempty" yaml:"maxAge,omitempty"`
}
```

### 3. Public API

#### Export Method on Router

```go
// ExportSpec returns a structured description of all registered routes.
func (r *Router[T, U]) ExportSpec() *ExportSpec

// ExportJSON writes the route spec to w as indented JSON.
func (r *Router[T, U]) ExportJSON(w io.Writer) error

// ExportJSONFile writes the route spec to the given file path as indented JSON.
func (r *Router[T, U]) ExportJSONFile(path string) error
```

These methods are safe to call after `NewRouter()` returns (all routes are registered during construction). They are read-only and safe for concurrent use.

#### Example Usage

```go
router, err := router.NewRouter[string, User](config)
if err != nil {
    log.Fatal(err)
}

// Export to file
if err := router.ExportJSONFile("api-spec.json"); err != nil {
    log.Fatal(err)
}

// Or export to stdout
router.ExportJSON(os.Stdout)
```

### 4. Metadata Collection Strategy

Route metadata is collected during the existing registration flow — no separate registration pass is needed.

**For standard routes (`RouteConfigBase`):**
- Path, methods, auth level, overrides, and timeout-disabled flag are captured directly from the config.
- No request/response schema (handler is an opaque `http.HandlerFunc`).

**For generic routes (`RouteConfig[Req, Resp]`):**
- All standard metadata is captured.
- Request and response type schemas are derived via `reflect.TypeOf` on the generic type parameters.
- The codec type is identified by a type assertion or a `Name() string` method added to the `Codec` interface.
- Source type and source key are captured for non-body request sources.
- The presence of a sanitizer function is noted (but not the function itself).

**For sub-routers:**
- The path prefix is recorded and prepended to child route paths.
- Sub-router-level overrides and auth level are resolved and applied.

**Implementation detail:** A new internal function builds a `RouteMetadata` from the resolved effective configuration (after applying the override hierarchy: global → sub-router → route). This ensures the export reflects what actually runs, not just what the user declared at one level.

### 5. Type Reflection Rules

When reflecting on request/response types to build `TypeSchema`:

1. **Structs:** Enumerate exported fields. For each field, record the Go name, the JSON tag name (if present), the type, and whether `omitempty` is absent (marking it as "required"). Recurse into nested struct fields.
2. **Slices/Arrays:** Record the element type. If the element is a struct, include its schema.
3. **Maps:** Record key and value types.
4. **Pointers:** Unwrap and describe the underlying type.
5. **Primitives:** Record the kind directly (`string`, `int`, `float64`, `bool`, etc.).
6. **Depth limit:** Recursion stops at depth 10 to handle any pathological circular type references. A `$ref`-style mechanism is not needed for Phase 1 but may be added later.

### 6. Output Formats

**Phase 1 scope:** JSON only. The structs carry both `json` and `yaml` tags so YAML support can be added trivially later via `gopkg.in/yaml.v3` without changing the data model.

JSON output uses `json.MarshalIndent` with 2-space indentation for readability.

### 7. Example Output

```json
{
  "version": "1.0",
  "exportedAt": "2025-03-25T12:00:00Z",
  "service": {
    "name": "my-api",
    "globalTimeout": "30s",
    "globalMaxBodySize": 1048576,
    "traceIdEnabled": true,
    "cors": {
      "origins": ["https://example.com"],
      "methods": ["GET", "POST", "PUT", "DELETE"],
      "headers": ["Content-Type", "Authorization"],
      "allowCredentials": true,
      "maxAge": "86400s"
    }
  },
  "routes": [
    {
      "path": "/api/v1/users",
      "methods": ["POST"],
      "authLevel": "required",
      "request": {
        "source": "body",
        "codec": "json",
        "schema": {
          "typeName": "CreateUserRequest",
          "package": "github.com/example/api/types",
          "kind": "struct",
          "fields": [
            {
              "name": "Name",
              "jsonName": "name",
              "type": "string",
              "required": true
            },
            {
              "name": "Email",
              "jsonName": "email",
              "type": "string",
              "required": true
            },
            {
              "name": "Age",
              "jsonName": "age",
              "type": "int",
              "required": false
            }
          ]
        },
        "hasSanitizer": true
      },
      "response": {
        "typeName": "CreateUserResponse",
        "package": "github.com/example/api/types",
        "kind": "struct",
        "fields": [
          {
            "name": "ID",
            "jsonName": "id",
            "type": "string",
            "required": true
          },
          {
            "name": "CreatedAt",
            "jsonName": "created_at",
            "type": "time.Time",
            "required": true
          }
        ]
      },
      "timeout": "10s",
      "maxBodySize": 524288,
      "rateLimit": {
        "bucketName": "create-user",
        "limit": 10,
        "window": "1m",
        "strategy": "ip"
      },
      "authToken": {
        "source": "header",
        "headerName": "Authorization"
      },
      "subRouter": "/api/v1"
    },
    {
      "path": "/api/v1/users/:id",
      "methods": ["GET"],
      "authLevel": "none",
      "request": {
        "source": "base64_query",
        "sourceKey": "filter",
        "codec": "json",
        "schema": {
          "typeName": "GetUserFilter",
          "package": "github.com/example/api/types",
          "kind": "struct",
          "fields": [
            {
              "name": "IncludeDeleted",
              "jsonName": "include_deleted",
              "type": "bool",
              "required": false
            }
          ]
        }
      },
      "response": {
        "typeName": "User",
        "package": "github.com/example/api/types",
        "kind": "struct",
        "fields": [
          {
            "name": "ID",
            "jsonName": "id",
            "type": "string",
            "required": true
          },
          {
            "name": "Name",
            "jsonName": "name",
            "type": "string",
            "required": true
          }
        ]
      },
      "subRouter": "/api/v1"
    },
    {
      "path": "/health",
      "methods": ["GET"],
      "authLevel": "none"
    }
  ]
}
```

Note: The last route (`/health`) is a standard (non-generic) route, so it has no request/response schema. The Phase 2 client will still render it as a sendable endpoint — the user just won't get a pre-filled request form.

---

## Phase 2: Client Application (Future — Out of Scope)

For context, the Phase 2 application will:

- **Import** the exported JSON spec file.
- **Render** a graphical interface listing all routes, grouped by sub-router prefix.
- **Present** a form for each route where the user can:
  - Select the HTTP method (from the allowed methods).
  - Fill in path parameters (detected from `:param` segments in the path).
  - Fill in request body fields using a form generated from the request schema, with field names, types, and required indicators pre-populated.
  - Set authentication headers/cookies based on the auth token metadata.
  - Configure a base URL for the target server.
- **Send** the constructed request and display the response, similar to Postman.

Phase 2 is not covered by this spec and will have its own design document.

---

## Implementation Plan (Phase 1)

### Step 1: Define Export Types

Create a new file `pkg/router/export.go` containing all the metadata structs (`ExportSpec`, `RouteMetadata`, `TypeSchema`, etc.).

### Step 2: Add Codec Identification

Add a `Name() string` method to the `codec.Codec` interface (returning `"json"` or `"proto"`). This is a breaking change to the interface, but both existing implementations (`JSONCodec`, `ProtoCodec`) will be updated in the same commit.

### Step 3: Add Reflection Utility

Create `pkg/router/reflect.go` with a function that takes a `reflect.Type` and returns a `*TypeSchema`. This handles struct field enumeration, tag parsing, recursion, and depth limiting.

### Step 4: Collect Metadata During Registration

Modify the internal route registration path to build a `RouteMetadata` for each route and append it to a `[]RouteMetadata` slice on the `Router` struct. This happens after the effective configuration (overrides, auth level, etc.) has been resolved, so the metadata reflects the actual runtime behavior.

### Step 5: Implement Export Methods

Implement `ExportSpec()`, `ExportJSON()`, and `ExportJSONFile()` on the `Router` struct. These are straightforward serialization of the collected metadata.

### Step 6: Tests

- Unit tests for type reflection (structs, nested structs, slices, maps, pointers, primitives, tag parsing).
- Unit tests for metadata collection (verify that routes registered via all paths — declarative, imperative, generic, standard — produce correct metadata).
- Integration test: build a router with representative routes, export to JSON, unmarshal, and assert the full structure.
- Golden file tests: compare export output against checked-in expected JSON files for regression detection.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Collect at registration time vs. runtime reflection | Registration time | Route configs are discarded after registration; this is the only point where all metadata is available. |
| Reflect on types vs. require user-provided schemas | Reflect | Zero additional effort for users; the types are already defined in Go code. |
| JSON first, YAML later | JSON | Simpler, no extra dependency. YAML tags are included on structs for easy future addition. |
| Flat route list vs. nested sub-router tree | Flat with `subRouter` field | Simpler for consumers; the Phase 2 client can group by prefix if desired. |
| Include function references in export | No | Functions are not serializable. We capture their *effects* (e.g., `hasSanitizer: true`, `strategy: "custom"`) instead. |
| Breaking change to Codec interface | Acceptable | Adding `Name()` is minimal; both implementations are in-tree and updated atomically. |
