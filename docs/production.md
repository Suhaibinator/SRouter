# Production Considerations

This guide covers important aspects of running SRouter applications in production, including performance optimization, security hardening, and graceful shutdown procedures.

## Performance Considerations

SRouter is designed with performance in mind, building upon the speed of `julienschmidt/httprouter`. Here are some factors and tips related to performance:

### Path Matching

SRouter inherits the high-performance path matching capabilities of `julienschmidt/httprouter`. This router uses a radix tree structure, allowing for path lookups that are generally O(k), where k is the length of the path, or even O(1) in many practical scenarios, significantly faster than routers relying solely on regular expressions for every route.

### Middleware Overhead

Each middleware layer introduces some runtime cost. Limit global and route-specific middleware to what you actually need. For a detailed explanation of how SRouter orders and applies middleware, see [Custom Middleware](./middleware.md#middleware-execution-order).

### Memory Allocation

SRouter aims to minimize memory allocations in the hot path (request processing). However, certain operations can still lead to allocations:

-   **Context Management**: While `SRouterContext` avoids deep nesting, adding values to the context still involves allocations.
-   **Logging**: Structured logging (like with `zap`) involves allocations for creating log entries and fields. Ensure your logger is configured appropriately for production (e.g., sampling, appropriate level) to minimize performance impact.
-   **Data Encoding/Decoding**: Codecs (JSON, Proto, etc.) inherently involve allocations for marshaling and unmarshaling data.
-   **Custom Middleware**: Be mindful of allocations within your own middleware functions.

**Tips for Reducing Allocations:**

-   Use `sync.Pool` for frequently allocated temporary objects within handlers or middleware if profiling shows significant allocation pressure.
-   Avoid unnecessary string formatting or concatenation within the request path.
-   Reuse objects like HTTP clients or database connections instead of creating them per request.

### Timeouts

Setting appropriate timeouts via `GlobalTimeout`, `SubRouterConfig.Overrides.Timeout`, and route-level `Overrides.Timeout` is crucial for both performance and stability:

-   Prevents slow client connections or long-running handlers from consuming server resources indefinitely.
-   Helps protect against certain types of Denial-of-Service (DoS) attacks.
-   Ensures predictable response times for clients.

Set timeouts based on the expected latency of the underlying operations for each route or group of routes.

### Body Size Limits

Configuring maximum request body sizes using `GlobalMaxBodySize`, `SubRouterConfig.Overrides.MaxBodySize`, and route-level `Overrides.MaxBodySize` is important for:

-   **Security**: Prevents DoS attacks where clients send excessively large request bodies to exhaust server memory or bandwidth.
-   **Performance**: Avoids processing unnecessarily large amounts of data.

Set limits based on the expected maximum payload size for each endpoint.

### Benchmarks

Consider running Go's built-in benchmarking tools (`go test -bench=.`) on your handlers and middleware to identify performance bottlenecks. SRouter itself likely includes benchmarks for its core routing and middleware components.

## Security Considerations

This section provides security-related guidance for developers using the SRouter framework. While SRouter aims to provide secure defaults and mechanisms, the ultimate security of your application depends on its proper configuration, your specific implementation choices, and adherence to general security best practices.

### IP Address Configuration

Correctly extracting the client IP is crucial for logging, rate limiting, and other security features. SRouter provides extensive configuration via `IPConfig` to handle proxies and custom headers. See the dedicated [IP Configuration](./ip-configuration.md) documentation for details and security recommendations.

### Authentication

SRouter provides a flexible authentication framework through its `pkg/middleware/auth.go` package. It includes several built-in providers and allows for custom authentication logic.

#### Available Authentication Providers

*   **`BearerTokenProvider`**: Authenticates requests based on a bearer token provided in the `Authorization` header. It can validate tokens against a predefined map or a custom validator function.
*   **`APIKeyProvider`**: Authenticates requests based on an API key that can be provided in a specified HTTP header or a URL query parameter. It validates keys against a predefined map.
*   **`BasicUserAuthProvider`**: Implements HTTP Basic Authentication. It extracts username and password from the `Authorization` header and uses a custom function (`GetUserFunc`) to validate credentials and retrieve a user object.
*   **`BearerTokenUserAuthProvider`**: Similar to `BearerTokenProvider`, but uses a custom function (`GetUserFunc`) to validate the token and retrieve a user object.
*   **`APIKeyUserAuthProvider`**: Similar to `APIKeyProvider`, but uses a custom function (`GetUserFunc`) to validate the API key and retrieve a user object.

The framework also provides generic middleware functions like `AuthenticationWithProvider`, `Authentication`, `AuthenticationBool`, `AuthenticationWithUserProvider`, and `AuthenticationWithUser` that can be used with these providers or with custom authentication functions.

#### Security Best Practices for Authentication

When implementing authentication, especially with custom validator functions (`Validator` in `BearerTokenProvider`, `GetUserFunc` in various `UserAuthProvider` implementations), it is crucial to follow security best practices:

*   **Secure Validation Logic:** Users are responsible for implementing secure token and credential validation logic within their custom functions.
    *   **Timing Attacks:** If you are comparing secrets, tokens, or API keys directly in your custom validation functions, it is highly recommended to use **constant-time comparison** functions. Standard string or byte slice comparisons (e.g., `==` operator or `bytes.Compare`) can be vulnerable to timing attacks, where an attacker can infer the value of a secret by measuring the time it takes for the comparison to complete. Go's `crypto/subtle` package provides functions like `subtle.ConstantTimeCompare` for this purpose.
    *   Using cryptographically strong random tokens (e.g., UUIDs, or tokens generated by a secure random number generator) as bearer tokens or API keys can mitigate the need for constant-time comparison if the tokens are sufficiently long and unique, as the primary check becomes existence in a database or a secure store, rather than direct comparison against a static secret.
*   **Secure Storage and Management:**
    *   API keys, tokens, and any other secrets should be stored securely. Avoid hardcoding them in source code. Use environment variables, configuration files with restricted access, or dedicated secret management systems (e.g., HashiCorp Vault, AWS Secrets Manager, Google Cloud Secret Manager).
    *   If storing password hashes, use a strong, adaptive hashing algorithm like bcrypt, scrypt, or Argon2.
    *   Implement proper key rotation policies for API keys and other long-lived credentials.
*   **HTTPS Everywhere:** Always use HTTPS (TLS) to protect credentials and session tokens while they are in transit between the client and the server. Transmitting credentials over plain HTTP can expose them to interception.
*   **Logging:** The authentication middleware functions in SRouter may accept a logger instance. While the default logging for authentication failures includes non-sensitive information like method, path, and remote address, be cautious if you customize logging within your authentication handlers or providers. Avoid logging sensitive information such as raw tokens, passwords, or full authorization headers unless they are appropriately masked or redacted.
*   **Error Messages:** The framework uses generic "Unauthorized" messages for authentication failures, which is good practice as it prevents leaking information about whether a username exists or if a password was incorrect. Maintain this practice in any custom error handling.
*   **Rate Limiting and Account Lockout:** Consider implementing rate limiting on authentication attempts and account lockout mechanisms to protect against brute-force attacks. (SRouter may provide separate middleware for rate limiting, which should be used in conjunction with authentication).
*   **Principle of Least Privilege:** Ensure that authenticated users or API keys only have the permissions necessary to perform their intended actions.

### Rate Limiting

SRouter includes a rate limiting middleware (`pkg/middleware/ratelimit.go`) to help protect your application from abuse, brute-force attacks, and Denial of Service (DoS) by limiting the number of requests a client can make to your API endpoints within a given time window.

#### Purpose of Rate Limiting

*   **Prevent Abuse:** Stop malicious actors from overwhelming your service with requests.
*   **Ensure Fair Usage:** Provide equitable access to resources for all users.
*   **Prevent DoS/DDoS:** Mitigate the impact of denial-of-service attacks by limiting the request rate from individual sources.
*   **Cost Control:** For services that incur costs per request, rate limiting can help manage expenses.

#### Available Strategies

The rate limiting middleware in SRouter supports several strategies for identifying clients:

*   **`StrategyIP`**: Limits requests based on the client's IP address.
    *   **Security Note:** The effectiveness and security of this strategy heavily depend on the correct configuration of IP address extraction. Please refer to the **"IP Address Configuration"** section of this document. If `TrustProxy` is incorrectly set to `true` in an environment where proxy headers can be spoofed, attackers might bypass IP-based rate limits by forging headers like `X-Forwarded-For`.
*   **`StrategyUser`**: Limits requests based on the authenticated user ID.
    *   **Security Note:** This strategy relies on the authentication middleware (see the **"Authentication"** section) to correctly and securely identify and authenticate users. If the authentication mechanism is weak or bypassed, user-based rate limiting will not be effective.
*   **`StrategyCustom`**: Allows for a user-defined `KeyExtractor` function to determine the unique key for rate limiting.
    *   **Security Note:** The security and effectiveness of this strategy depend entirely on the implementation of the custom `KeyExtractor` function. Ensure that the extracted key accurately represents the entity you wish to rate limit and cannot be easily manipulated by attackers to bypass limits or affect other users.

#### Memory Consumption Considerations

SRouter's default rate limiter, `UberRateLimiter` (which uses `go.uber.org/ratelimit`), stores rate limiting state in memory for each unique key (e.g., each IP address or user ID).

*   **High Cardinality Warning:** If your application anticipates a very large number of unique keys (e.g., millions of different IP addresses making requests over a short period, or a vast number of user accounts being actively rate-limited simultaneously) without application restarts, this in-memory storage can lead to significant memory consumption.
*   **Suitability:** The current `UberRateLimiter` is well-suited for many common use cases, especially for applications with a moderate number of active users or for services deployed behind a load balancer that already handles a significant portion of traffic.
*   **Alternatives for Extreme Scale:** For extremely high-traffic scenarios with very high cardinality of rate limiting keys, or if you require persistence of rate limiting state across application restarts or a distributed environment, consider:
    *   Using an external rate-limiting solution (e.g., Redis-based, or dedicated proxy/API gateway rate limiters).
    *   Implementing a custom `RateLimiter` for SRouter that uses a backend store with eviction policies (e.g., LRU cache, Redis with TTLs) to manage memory usage.

#### Standard Rate Limit Headers

When a request is subject to rate limiting, SRouter's middleware will typically add the following standard HTTP headers to the response:

*   **`X-RateLimit-Limit`**: The maximum number of requests allowed in the current time window.
*   **`X-RateLimit-Remaining`**: The number of requests remaining in the current time window.
*   **`X-RateLimit-Reset`**: The time (in UTC seconds since epoch) when the rate limit will reset.
*   **`Retry-After`**: If the limit is exceeded, this header may be included, indicating how many seconds the client should wait before making another request. The value will correspond to when the current window resets.

These headers allow clients to understand the rate limits and adjust their request patterns accordingly.

### Input Validation and Sanitization

While SRouter provides robust mechanisms for request routing, middleware execution, encoding/decoding of request/response bodies (e.g., JSON), and setting limits like maximum request body size, the detailed validation of input *content* and the sanitization of data to prevent attacks like Cross-Site Scripting (XSS) are primarily the **developer's responsibility** within their route handlers.

SRouter itself does not perform deep inspection or validation of the data fields within request bodies or parameters beyond basic parsing and type conversion where applicable.

#### SRouter's Built-in Mechanisms

*   **Body Size Limits:** The `RouteConfig` allows setting a `MaxBodyBytes` to limit the size of incoming request bodies, which can help prevent certain types of denial-of-service attacks caused by excessively large payloads.
*   **Generic Route Sanitizer:** For routes registered using `router.RegisterGenericRoute`, the `RouteConfig` accepts an optional `Sanitizer` function with the signature `func(T) (T, error)`.
    *   This function is executed *before* the main handler and *after* the request body has been read (up to `MaxBodyBytes`).
    *   It can be used to perform both **sanitization** (modifying the input to remove malicious parts) and **validation** (checking if the input conforms to expected formats, ranges, or patterns).
    *   If the `Sanitizer` function returns an error, the request is typically rejected by the framework with an appropriate HTTP error code (e.g., `400 Bad Request`), and the main handler is not called.
    *   Refer to the example at `examples/generic/main.go` for a demonstration of how to implement and use a `Sanitizer` function.

#### Developer Responsibilities and Recommendations

It is strongly recommended that developers implement robust input validation and sanitization for all incoming data sources within their application logic:

*   **Request Bodies:**
    *   Validate the structure and data types of JSON/XML payloads.
    *   Check for required fields, string lengths, numerical ranges, enumeration values, and specific formats (e.g., email addresses, UUIDs).
*   **Query Parameters:**
    *   Validate the type and format of each query parameter.
    *   Be mindful of parameters used in database queries or other sensitive operations.
*   **Path Parameters:**
    *   While the underlying `httprouter` (or a similar routing library) handles the basic matching of URL paths, the actual content of a path parameter might still require validation. For example, if a path parameter is expected to be a numeric ID, ensure it is indeed a number and within an acceptable range.
*   **HTTP Headers:**
    *   Validate any custom or standard HTTP headers that your application relies on for its logic.

#### Tools and Libraries

*   Consider using popular Go libraries for validation to simplify and standardize your validation logic. For instance, `go-playground/validator` is widely used for struct-based validation using tags, allowing you to define validation rules directly in your data structures.

#### Common Vulnerabilities to Mitigate

While SRouter itself may not directly cause or prevent these, developers should be mindful of common web application vulnerabilities related to input/output handling:

*   **Cross-Site Scripting (XSS):**
    *   If your application generates HTML content dynamically based on user input, ensure that all user-supplied data is properly sanitized or escaped before being rendered.
    *   Using Go's `html/template` package provides context-aware auto-escaping, which is highly effective against XSS when used correctly.
    *   Set appropriate `Content-Security-Policy` (CSP) headers to further restrict the capabilities of scripts in the browser.
*   **SQL Injection (SQLi):**
    *   If your application interacts with SQL databases, always use parameterized queries or prepared statements.
    *   Avoid constructing SQL queries by concatenating user-supplied strings directly.
    *   Use an ORM (like GORM, used in some SRouter examples) that handles SQL parameterization, but still be mindful of how you construct queries with user input even when using an ORM.
*   **Command Injection:** If user input is used to construct shell commands or arguments, ensure it is properly sanitized to prevent attackers from executing arbitrary commands on the server.
*   **Directory Traversal:** If user input is used to construct file paths, validate and sanitize it to prevent access to unintended files or directories.

By diligently validating and sanitizing all inputs, and by being aware of common output encoding needs, developers can significantly improve the security posture of their SRouter-based applications.

### Cross-Origin Resource Sharing (CORS)

Cross-Origin Resource Sharing (CORS) is a browser security feature that restricts web pages from making requests to a different domain (origin) than the one that served the web page. This is a crucial security measure to prevent malicious websites from reading sensitive data from other websites or performing unauthorized actions on behalf of the user.

SRouter provides a built-in CORS middleware that can be configured through the `CORSConfig` field in the main `RouterConfig`. This middleware automatically handles preflight `OPTIONS` requests and adds the necessary CORS headers to responses.

#### CORS Configuration Options

The `CORSConfig` struct within `RouterConfig` offers the following options to control CORS behavior:

*   **`Origins` (`[]string`):** A list of allowed origins. An origin is a combination of scheme (e.g., `http`, `https`), domain, and port.
    *   Example: `[]string{"https://www.example.com", "https://api.example.com"}`
    *   A wildcard `"*"` can be used to allow all origins. However, see the security implications below.
*   **`Methods` (`[]string`):** A list of allowed HTTP methods for cross-origin requests (e.g., `"GET"`, `"POST"`, `"PUT"`, `"DELETE"`).
*   **`Headers` (`[]string`):** A list of allowed HTTP headers that the client can send in a cross-origin request.
*   **`AllowCredentials` (`bool`):** If set to `true`, the browser will allow cookies and other credentials (like HTTP Basic authentication) to be sent with cross-origin requests.
    *   **Security Note:** This requires careful consideration and alignment with `Origins` settings.
*   **`MaxAge` (`int`):** Specifies how long the results of a preflight request (an `OPTIONS` request) can be cached by the browser, in seconds.
*   **`ExposeHeaders` (`[]string`):** A list of response headers that the browser should allow client-side JavaScript to access. By default, many headers are not exposed.

#### Security Implications and Best Practices

*   **`Origins` and Wildcards (`"*"`):**
    *   Using a wildcard `"*"` for `Origins` is the most permissive setting, allowing any domain to make requests to your API. While this is flexible, it may not always be the most secure choice.
    *   **Recommendation:** For better security, explicitly list the origins that are permitted to access your resources. This is especially important for applications handling sensitive data.
    *   **Important:** If `AllowCredentials` is set to `true`, the `Access-Control-Allow-Origin` header *cannot* be a wildcard (`"*"`). SRouter's CORS middleware correctly handles this by ensuring that if `AllowCredentials` is true and `Origins` contains `*`, it will dynamically set the `Access-Control-Allow-Origin` to the specific origin of the request if that origin is found within the configured `Origins` list (excluding the wildcard itself for this specific header when credentials are allowed). If no specific match is found, the request may be rejected or the credential-related headers might not be set appropriately.

*   **`AllowCredentials` (`bool`):**
    *   Setting `AllowCredentials` to `true` means that the server is willing to process cookies or other user credentials sent by the browser with cross-origin requests.
    *   **Security Warning:** Only enable this if your application specifically requires credentialed cross-origin requests (e.g., for session management with a separate frontend domain).
    *   When `AllowCredentials` is `true`, the `Access-Control-Allow-Origin` header must be a specific origin, not a wildcard. Ensure your `Origins` list is configured accordingly.

*   **`ExposeHeaders` (`[]string`):**
    *   By default, browsers restrict access to many response headers for client-side JavaScript. If your frontend needs to read specific headers from cross-origin responses (e.g., a custom pagination header or a rate limit header), you must include them in the `ExposeHeaders` list.
    *   Only expose headers that are strictly necessary for the client application to function.

*   **Handling of Preflight `OPTIONS` Requests:** SRouter's CORS middleware automatically handles preflight `OPTIONS` requests. These requests are sent by the browser before the actual request (e.g., `GET`, `POST`) to determine if the server permits the cross-origin communication with the specified method and headers.

*   **Restrictive Policies:**
    *   **Recommendation:** Always configure your CORS policy to be as restrictive as possible while still allowing your legitimate frontend applications to function correctly.
    *   Avoid using wildcards for `Origins`, `Methods`, or `Headers` unless absolutely necessary and the security implications are fully understood.
    *   Regularly review your CORS configuration to ensure it aligns with your application's current needs and security posture.

By carefully configuring these options, you can ensure that your SRouter application interacts securely with web browsers from different origins.

### General Security Best Practices

Building a secure application is a multifaceted endeavor that goes beyond the scope of a single framework. While SRouter provides tools to help you build secure web applications, remember the following general principles:

*   **Keep Dependencies Up-to-Date:** Regularly update SRouter and all other third-party libraries to their latest stable versions. Security vulnerabilities are often discovered and patched in newer releases. Use tools like `go list -u -m all` and `go get` to manage your Go module dependencies.
*   **Secure Deployment Environment:** Ensure your deployment environment (servers, containers, operating systems, network configurations) is hardened and configured securely.
*   **Handle Sensitive Data with Care:**
    *   Minimize the amount of sensitive data you collect and store.
    *   Encrypt sensitive data at rest and in transit (HTTPS).
    *   Implement appropriate access controls for sensitive information.
    *   Be mindful of logging sensitive data; redact or avoid logging it whenever possible.
*   **Regular Security Testing:**
    *   Conduct regular security audits and penetration testing of your application.
    *   Utilize static analysis security testing (SAST) and dynamic analysis security testing (DAST) tools.
    *   Consider implementing a bug bounty program if appropriate for your application.
*   **Follow Secure Coding Practices:** Adhere to general secure coding principles, such as the OWASP Top Ten, to avoid common web application vulnerabilities.
*   **Defense in Depth:** Employ multiple layers of security controls. No single security measure is foolproof.
*   **Stay Informed:** Keep abreast of the latest security threats and best practices in web application development.

By combining the secure features of SRouter with these general best practices, you can significantly enhance the security posture of your applications.

## Graceful Shutdown

Properly shutting down your HTTP server is crucial to avoid abruptly terminating active connections and potentially losing data or leaving operations in an inconsistent state. SRouter provides mechanisms to support graceful shutdown.

### Router Shutdown Method

The SRouter instance (`*router.Router`) itself has a `Shutdown` method. Calling this marks the router as shutting down so no new requests are served, waits for any in‑flight requests to finish, and stops internal components such as the trace ID generator (if enabled).

```go
// func (r *Router[T, U]) Shutdown(ctx context.Context) error
```

It takes a `context.Context` which can be used to set a deadline for the shutdown process.

### Integrating with `http.Server` Shutdown

The standard Go `net/http` package provides the `http.Server.Shutdown` method for gracefully shutting down the server. This method stops the server from accepting new connections and waits for active requests to complete before returning.

You should call both the `http.Server.Shutdown` and the `router.Router.Shutdown` methods when handling termination signals (like `SIGINT` or `SIGTERM`).

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall" // Use syscall for SIGTERM if needed
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router" // Assuming router setup as 'r'
	"go.uber.org/zap"                           // Assuming logger setup as 'logger'
)

func main() {
	// --- Assume router 'r' is configured and created here ---
	logger, _ := zap.NewProduction() // Example logger
	defer logger.Sync()
	// ... router config ...
	r := router.NewRouter[string, string](/* ... config, auth funcs ... */)
	// ... register routes ...

	// Create the HTTP server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r, // Use the SRouter instance as the handler
		// Add other server configurations like ReadTimeout, WriteTimeout if desired
	}

	// Start the server in a separate goroutine so it doesn't block
	go func() {
		log.Println("Server starting on", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe failed: %v", err)
		}
	}()

	// Wait for interrupt signal (SIGINT) or termination signal (SIGTERM)
	quit := make(chan os.Signal, 1)
	// signal.Notify(quit, os.Interrupt) // Catch only Ctrl+C
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM) // Catch Ctrl+C and SIGTERM
	receivedSignal := <-quit
	log.Printf("Received signal: %s. Shutting down server...\n", receivedSignal)

	// Create a context with a timeout for the shutdown process
	// Give active requests time to finish (e.g., 5 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// --- Perform Shutdown ---

        // 1. Shut down the SRouter instance (stops new requests and halts internal components like the trace ID generator)
	// It's generally good practice to shut down the router logic first.
	if err := r.Shutdown(ctx); err != nil {
		log.Printf("SRouter shutdown failed: %v\n", err)
		// Decide if this error is fatal or if you should proceed
	} else {
		log.Println("SRouter shut down successfully.")
	}

	// 2. Shut down the HTTP server (stops accepting new connections, waits for active requests)
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP server shutdown failed: %v", err) // Often fatal if server can't shut down
	} else {
		log.Println("HTTP server shut down successfully.")
	}

	log.Println("Server exited gracefully")
}

```

### Shutdown Order

1.  **Receive Signal**: Detect `SIGINT` or `SIGTERM`.
2.  **Create Context**: Create a `context.WithTimeout` to limit the shutdown duration.
3.  **Router Shutdown**: Call `r.Shutdown(ctx)` to signal SRouter's internal components.
4.  **Server Shutdown**: Call `srv.Shutdown(ctx)` to stop accepting new connections and wait for existing requests handled by SRouter to complete.

This ensures that the server stops accepting new work, SRouter components are notified, and active requests are given time to finish before the application exits.

See the `examples/graceful-shutdown` directory for a runnable example.