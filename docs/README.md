# SRouter Documentation

Welcome to the official documentation for SRouter, a high-performance HTTP router for Go.

[![Go Report Card](https://goreportcard.com/badge/github.com/Suhaibinator/SRouter)](https://goreportcard.com/report/github.com/Suhaibinator/SRouter)
[![GoDoc](https://godoc.org/github.com/Suhaibinator/SRouter?status.svg)](https://godoc.org/github.com/Suhaibinator/SRouter)
[![Tests](https://github.com/Suhaibinator/SRouter/actions/workflows/tests.yml/badge.svg)](https://github.com/Suhaibinator/SRouter/actions/workflows/tests.yml)
[![codecov](https://codecov.io/gh/Suhaibinator/SRouter/graph/badge.svg?token=NNIYO5HKX7)](https://codecov.io/gh/Suhaibinator/SRouter)

## Overview

SRouter is a high-performance HTTP router for Go built upon [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter). It enhances the core routing capabilities with advanced features designed for building robust and scalable web services. Key additions include sub-router overrides, sophisticated middleware support, generic-based request/response handling, configurable timeouts and body size limits, flexible authentication levels, a comprehensive metrics system, and intelligent logging with trace ID support.

## Table of Contents

- **Getting Started**
  - [Installation & Requirements](./installation.md)
  - [Basic Usage](./basic-usage.md)
  - [Sub-Routers](./subrouters.md)
  - [Generic Routes](./generic-routes.md)
  - [Path Parameters](./path-parameters.md)
  - [Trace ID Logging](./trace-logging.md)
  - [Graceful Shutdown](./graceful-shutdown.md)
- **Advanced Features**
  - [IP Configuration](./ip-configuration.md)
  - [Rate Limiting](./rate-limiting.md)
  - [Authentication](./authentication.md)
  - [Context Management](./context-management.md)
  - [Custom Error Handling](./error-handling.md)
  - [Custom Middleware](./middleware.md)
  - [Source Types](./source-types.md)
  - [Custom Codecs](./codecs.md)
  - [Metrics](./metrics.md)
- **Reference**
  - [Configuration Reference](./configuration.md)
  - [Middleware Reference](./middleware.md#middleware-reference)
  - [Codec Reference](./codecs.md#codec-reference)
  - [Path Parameter Reference](./path-parameters.md#path-parameter-reference)
  - [Error Handling Reference](./error-handling.md#error-handling-reference)
- **Other**
  - [Performance Considerations](./performance.md)
  - [Logging](./logging.md)
  - [Examples](./examples.md)
  - [License](./license.md)
