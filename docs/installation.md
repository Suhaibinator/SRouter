# Installation & Requirements

## Installation

To install SRouter, use `go get`:

```bash
go get github.com/Suhaibinator/SRouter
```

## Requirements

- Go 1.24.0 or higher
- Dependencies (managed via Go modules):
  - [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) v1.3.0+ (Core routing engine)
  - [go.uber.org/zap](https://github.com/uber-go/zap) v1.27.0+ (Structured logging)
  - [github.com/google/uuid](https://github.com/google/uuid) v1.6.0+ (Used internally, e.g., trace IDs)
  - [go.uber.org/ratelimit](https://github.com/uber-go/ratelimit) v0.3.1+ (Used by built-in rate limiting middleware)
  - [google.golang.org/protobuf](https://github.com/protocolbuffers/protobuf-go) (Required if using `ProtoCodec`)
  - [gorm.io/gorm](https://gorm.io/) (Required if using `GormTransactionWrapper` or related DB features)
  - Metrics dependencies (e.g., [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang)) if using the metrics system.

All required dependencies will be automatically downloaded and installed when you run `go get github.com/Suhaibinator/SRouter`.
