package router

// RegisterSubRouter registers a sub-router with the router.
// It applies the sub-router's path prefix to all routes and registers them with the router.
// This is an exported wrapper for the unexported registerSubRouter method.
func (r *Router[T, U]) RegisterSubRouter(sr SubRouterConfig) {
	r.registerSubRouter(sr)
}
