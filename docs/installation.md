# Installation & Requirements

## Installation

To install SRouter, use `go get`:

```bash
go get github.com/Suhaibinator/SRouter
```

## Requirements

- Go 1.24.0 or higher
- Dependencies (managed via Go modules):
  - [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) v1.3.0+ (for high-performance routing)
  - [go.uber.org/zap](https://github.com/uber-go/zap) v1.27.0+ (for structured logging)
  - [github.com/google/uuid](https://github.com/google/uuid) v1.6.0+ (for trace ID generation)
  - [go.uber.org/ratelimit](https://github.com/uber-go/ratelimit) v0.3.1+ (Optional, used by rate limiting middleware)
  - Metrics dependencies (e.g., [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang)) if using the metrics system.

All required dependencies will be automatically downloaded and installed when you run `go get github.com/Suhaibinator/SRouter`.
