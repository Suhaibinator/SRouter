package scontext

import "context"

// ClearUserID removes the stored user ID value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearUserID[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	var zero T
	rc.userID = zero
	rc.presence &^= presentUserID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearUser removes the stored user object value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearUser[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.user = nil
	rc.presence &^= presentUser
	rc.mu.Unlock()
	return ctx
}

// ClearIdentity removes the user ID and user object.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearIdentity[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	var zero T
	rc.userID = zero
	rc.user = nil
	rc.presence &^= presentUserID | presentUser
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearBuildID removes the stored build identity value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearBuildID[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.buildID = ""
	rc.presence &^= presentBuildID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearConfigID removes the stored configuration identity value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearConfigID[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.configID = ""
	rc.presence &^= presentConfigID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearClientIP removes the stored client IP value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearClientIP[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.clientIP = ""
	rc.presence &^= presentClientIP
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearUserAgent removes the stored user agent value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearUserAgent[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.userAgent = ""
	rc.presence &^= presentUserAgent
	rc.mu.Unlock()
	return ctx
}

// ClearClientInfo removes the client IP and user agent.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearClientInfo[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.clientIP = ""
	rc.userAgent = ""
	rc.presence &^= presentClientIP | presentUserAgent
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearTraceID removes the stored trace ID value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearTraceID[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.traceID = ""
	rc.presence &^= presentTraceID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// ClearRouteInfo removes the route template and path parameters.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearRouteInfo[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.routeTemplate = ""
	rc.pathParams = nil
	rc.presence &^= presentRouteTemplate
	rc.mu.Unlock()
	return ctx
}

// ClearCORSInfo removes the allowed origin and credentials setting.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearCORSInfo[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.allowedOrigin = ""
	rc.credentialsAllowed = false
	rc.presence &^= presentAllowedOrigin | presentCredentialsAllowed
	rc.mu.Unlock()
	return ctx
}

// ClearCORSRequestedHeaders removes the stored requested CORS headers value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearCORSRequestedHeaders[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.requestedHeaders = ""
	rc.presence &^= presentRequestedHeaders
	rc.mu.Unlock()
	return ctx
}

// ClearHandlerError removes the stored handler error value and its presence.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearHandlerError[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.handlerError = nil
	rc.presence &^= presentHandlerError
	rc.mu.Unlock()
	return ctx
}

// ClearFlag removes the named application flag.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
func ClearFlag[T comparable, U any](ctx context.Context, name string) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	delete(rc.flags, name)
	rc.mu.Unlock()
	return ctx
}

// ClearRequestLogger removes the logging source and cached request logger.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Use CopySRouterContext first when
// parent or sibling operations must retain their values.
// Subsequent GetLogger calls reflect the removal; previously returned loggers
// remain immutable snapshots.
func ClearRequestLogger[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.logSource = nil
	rc.logger = nil
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}
