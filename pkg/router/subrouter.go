package router

// RegisterSubRouter registers a sub-router with the router after router creation.
// This method allows dynamic registration of sub-routers at runtime.
//
// The sub-router's configuration is applied as follows:
// - Path prefix is prepended to all routes in the sub-router
// - Middlewares are added to (not replacing) global middlewares
// - Configuration overrides (timeout, max body size, rate limit) apply only to direct routes
// - Nested sub-routers will have their path prefixes concatenated but must set their own overrides
//
// This is useful for conditionally adding routes or building routes programmatically.
func (r *Router[T, U]) RegisterSubRouter(sr SubRouterConfig) {
	r.registerSubRouter(sr)
}
