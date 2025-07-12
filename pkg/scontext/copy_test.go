package scontext

import (
	"context"
	"errors"
	"testing"

	"github.com/julienschmidt/httprouter"
	"gorm.io/gorm"
)

type mockTransaction struct{}

func (m *mockTransaction) Commit() error                { return nil }
func (m *mockTransaction) Rollback() error              { return nil }
func (m *mockTransaction) SavePoint(name string) error  { return nil }
func (m *mockTransaction) RollbackTo(name string) error { return nil }
func (m *mockTransaction) GetDB() *gorm.DB              { return nil }

type testUser struct {
	ID   int
	Name string
}

// createFullSRouterContext creates a context with all possible SRouter values set.
// This is a test helper to avoid code duplication.
func createFullSRouterContext() context.Context {
	ctx := context.Background()
	userID := 123
	user := &testUser{ID: 456, Name: "John Doe"}
	traceID := "trace-123"
	clientIP := "192.168.1.1"
	userAgent := "test-agent"
	tx := &mockTransaction{}
	routeTemplate := "/users/:id"
	params := httprouter.Params{
		{Key: "id", Value: "123"},
		{Key: "name", Value: "test"},
	}
	allowedOrigin := "https://example.com"
	credentialsAllowed := true
	requestedHeaders := "X-Custom-Header"
	handlerErr := errors.New("test error")

	// Set all values in context
	ctx = WithUserID[int, testUser](ctx, userID)
	ctx = WithUser[int, testUser](ctx, user)
	ctx = WithTraceID[int, testUser](ctx, traceID)
	ctx = WithClientIP[int, testUser](ctx, clientIP)
	ctx = WithUserAgent[int, testUser](ctx, userAgent)
	ctx = WithTransaction[int, testUser](ctx, tx)
	ctx = WithRouteInfo[int, testUser](ctx, params, routeTemplate)
	ctx = WithCORSInfo[int, testUser](ctx, allowedOrigin, credentialsAllowed)
	ctx = WithCORSRequestedHeaders[int, testUser](ctx, requestedHeaders)
	ctx = WithHandlerError[int, testUser](ctx, handlerErr)
	ctx = WithFlag[int, testUser](ctx, "test-flag", true)
	ctx = WithFlag[int, testUser](ctx, "another-flag", false)

	return ctx
}

// verifyFullSRouterContext verifies that a context contains all expected SRouter values.
// This is a test helper to avoid code duplication.
func verifyFullSRouterContext(t *testing.T, ctx context.Context, testName string) {
	t.Helper()

	userID := 123
	user := &testUser{ID: 456, Name: "John Doe"}
	traceID := "trace-123"
	clientIP := "192.168.1.1"
	userAgent := "test-agent"
	routeTemplate := "/users/:id"
	params := httprouter.Params{
		{Key: "id", Value: "123"},
		{Key: "name", Value: "test"},
	}
	allowedOrigin := "https://example.com"
	credentialsAllowed := true
	requestedHeaders := "X-Custom-Header"
	handlerErr := errors.New("test error")

	// Verify all values were copied correctly
	copiedUserID, ok := GetUserID[int, testUser](ctx)
	if !ok || copiedUserID != userID {
		t.Errorf("%s: UserID not copied correctly. Expected %d, got %d (ok: %v)", testName, userID, copiedUserID, ok)
	}

	copiedUser, ok := GetUser[int, testUser](ctx)
	if !ok || copiedUser == nil || copiedUser.ID != user.ID || copiedUser.Name != user.Name {
		t.Errorf("%s: User not copied correctly. Expected %+v, got %+v (ok: %v)", testName, user, copiedUser, ok)
	}

	copiedTraceID := GetTraceIDFromContext[int, testUser](ctx)
	if copiedTraceID != traceID {
		t.Errorf("%s: TraceID not copied correctly. Expected %s, got %s", testName, traceID, copiedTraceID)
	}

	copiedClientIP, ok := GetClientIP[int, testUser](ctx)
	if !ok || copiedClientIP != clientIP {
		t.Errorf("%s: ClientIP not copied correctly. Expected %s, got %s (ok: %v)", testName, clientIP, copiedClientIP, ok)
	}

	copiedUserAgent, ok := GetUserAgent[int, testUser](ctx)
	if !ok || copiedUserAgent != userAgent {
		t.Errorf("%s: UserAgent not copied correctly. Expected %s, got %s (ok: %v)", testName, userAgent, copiedUserAgent, ok)
	}

	copiedTx, ok := GetTransaction[int, testUser](ctx)
	if !ok || copiedTx == nil {
		t.Errorf("%s: Transaction not copied correctly. Expected non-nil, got %v (ok: %v)", testName, copiedTx, ok)
	}

	copiedRouteTemplate, ok := GetRouteTemplateFromContext[int, testUser](ctx)
	if !ok || copiedRouteTemplate != routeTemplate {
		t.Errorf("%s: RouteTemplate not copied correctly. Expected %s, got %s (ok: %v)", testName, routeTemplate, copiedRouteTemplate, ok)
	}

	copiedParams, ok := GetPathParamsFromContext[int, testUser](ctx)
	if !ok || len(copiedParams) != len(params) {
		t.Errorf("%s: PathParams not copied correctly. Expected %v, got %v (ok: %v)", testName, params, copiedParams, ok)
	}
	for i, param := range params {
		if copiedParams[i].Key != param.Key || copiedParams[i].Value != param.Value {
			t.Errorf("%s: PathParams[%d] not copied correctly. Expected %v, got %v", testName, i, param, copiedParams[i])
		}
	}

	copiedOrigin, copiedCreds, ok := GetCORSInfo[int, testUser](ctx)
	if !ok || copiedOrigin != allowedOrigin || copiedCreds != credentialsAllowed {
		t.Errorf("%s: CORS info not copied correctly. Expected (%s, %v), got (%s, %v) (ok: %v)",
			testName, allowedOrigin, credentialsAllowed, copiedOrigin, copiedCreds, ok)
	}

	copiedHeaders, ok := GetCORSRequestedHeaders[int, testUser](ctx)
	if !ok || copiedHeaders != requestedHeaders {
		t.Errorf("%s: CORS requested headers not copied correctly. Expected %s, got %s (ok: %v)", testName, requestedHeaders, copiedHeaders, ok)
	}

	copiedHandlerErr, ok := GetHandlerError[int, testUser](ctx)
	if !ok || copiedHandlerErr.Error() != handlerErr.Error() {
		t.Errorf("%s: Handler error not copied correctly. Expected %v, got %v (ok: %v)", testName, handlerErr, copiedHandlerErr, ok)
	}

	copiedFlag1, ok := GetFlag[int, testUser](ctx, "test-flag")
	if !ok || !copiedFlag1 {
		t.Errorf("%s: Flag 'test-flag' not copied correctly. Expected true, got %v (ok: %v)", testName, copiedFlag1, ok)
	}

	copiedFlag2, ok := GetFlag[int, testUser](ctx, "another-flag")
	if !ok || copiedFlag2 {
		t.Errorf("%s: Flag 'another-flag' not copied correctly. Expected false, got %v (ok: %v)", testName, copiedFlag2, ok)
	}
}

func TestCopySRouterContext(t *testing.T) {
	srcCtx := createFullSRouterContext()
	dstCtx := context.Background()

	copiedCtx := CopySRouterContext[int, testUser](dstCtx, srcCtx)

	verifyFullSRouterContext(t, copiedCtx, "CopySRouterContext")
}

func TestCopySRouterContext_NoSourceContext(t *testing.T) {
	// Source context without SRouterContext
	srcCtx := context.Background()
	dstCtx := context.Background()

	// Copy should return destination unchanged
	copiedCtx := CopySRouterContext[int, testUser](dstCtx, srcCtx)

	if copiedCtx != dstCtx {
		t.Error("Expected destination context to be returned unchanged when source has no SRouterContext")
	}

	// Verify no SRouterContext was added
	_, ok := GetSRouterContext[int, testUser](copiedCtx)
	if ok {
		t.Error("Expected no SRouterContext to be present in destination")
	}
}

func TestCopySRouterContext_Independence(t *testing.T) {
	// Create source context with flags
	srcCtx := context.Background()
	srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", true)

	// Copy to destination
	dstCtx := context.Background()
	copiedCtx := CopySRouterContext[int, testUser](dstCtx, srcCtx)

	// Modify flag in source
	srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", false)
	srcCtx = WithFlag[int, testUser](srcCtx, "source-only", true)

	// Modify flag in copied context
	copiedCtx = WithFlag[int, testUser](copiedCtx, "shared-flag", true) // Keep original value
	copiedCtx = WithFlag[int, testUser](copiedCtx, "copied-only", true)

	// Verify independence
	srcFlag, ok := GetFlag[int, testUser](srcCtx, "shared-flag")
	if !ok || srcFlag {
		t.Errorf("Source context flag should be false, got %v (ok: %v)", srcFlag, ok)
	}

	copiedFlag, ok := GetFlag[int, testUser](copiedCtx, "shared-flag")
	if !ok || !copiedFlag {
		t.Errorf("Copied context flag should be true, got %v (ok: %v)", copiedFlag, ok)
	}

	// Verify source-only flag doesn't exist in copied context
	_, ok = GetFlag[int, testUser](copiedCtx, "source-only")
	if ok {
		t.Error("Source-only flag should not exist in copied context")
	}

	// Verify copied-only flag doesn't exist in source context
	_, ok = GetFlag[int, testUser](srcCtx, "copied-only")
	if ok {
		t.Error("Copied-only flag should not exist in source context")
	}
}

func TestCopySRouterContextMerge(t *testing.T) {
	t.Run("FullCopy", func(t *testing.T) {
		srcCtx := createFullSRouterContext()
		dstCtx := context.Background()

		copiedCtx := CopySRouterContextMerge[int, testUser](dstCtx, srcCtx)

		verifyFullSRouterContext(t, copiedCtx, "CopySRouterContextMerge")
	})

	t.Run("NoSourceContext", func(t *testing.T) {
		srcCtx := context.Background()
		dstCtx := context.Background()

		copiedCtx := CopySRouterContextMerge[int, testUser](dstCtx, srcCtx)

		if copiedCtx != dstCtx {
			t.Error("Expected destination context to be returned unchanged when source has no SRouterContext")
		}

		_, ok := GetSRouterContext[int, testUser](copiedCtx)
		if ok {
			t.Error("Expected no SRouterContext to be present in destination")
		}
	})

	t.Run("DestinationAlreadyHasContext", func(t *testing.T) {
		srcCtx := createFullSRouterContext()

		// Create destination with some existing context
		dstCtx := context.Background()
		dstCtx = WithUserID[int, testUser](dstCtx, 999)
		dstCtx = WithFlag[int, testUser](dstCtx, "dst-flag", true)

		copiedCtx := CopySRouterContextMerge[int, testUser](dstCtx, srcCtx)

		// Should have source values, not destination values
		verifyFullSRouterContext(t, copiedCtx, "CopySRouterContextMerge_Overwrite")

		// Original destination flag should be overwritten
		_, ok := GetFlag[int, testUser](copiedCtx, "dst-flag")
		if ok {
			t.Error("Destination flag should have been overwritten")
		}
	})

	t.Run("Independence", func(t *testing.T) {
		srcCtx := context.Background()
		srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", true)

		dstCtx := context.Background()
		copiedCtx := CopySRouterContextMerge[int, testUser](dstCtx, srcCtx)

		// Modify source after copy
		srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", false)
		srcCtx = WithFlag[int, testUser](srcCtx, "source-only", true)

		// Modify copied after copy
		copiedCtx = WithFlag[int, testUser](copiedCtx, "copied-only", true)

		// Verify independence
		srcFlag, _ := GetFlag[int, testUser](srcCtx, "shared-flag")
		copiedFlag, _ := GetFlag[int, testUser](copiedCtx, "shared-flag")

		if srcFlag || !copiedFlag {
			t.Errorf("Contexts should be independent. Source flag: %v, Copied flag: %v", srcFlag, copiedFlag)
		}
	})
}

func TestCopySRouterContextOverlay(t *testing.T) {
	t.Run("FullCopy_DestinationHasContext", func(t *testing.T) {
		srcCtx := createFullSRouterContext()

		// Create destination with existing context
		dstCtx := context.Background()
		dstCtx = WithUserID[int, testUser](dstCtx, 999)
		dstCtx = WithFlag[int, testUser](dstCtx, "dst-flag", true)

		copiedCtx := CopySRouterContextOverlay[int, testUser](dstCtx, srcCtx)

		// Should have source values, not destination values
		verifyFullSRouterContext(t, copiedCtx, "CopySRouterContextOverlay")

		// Original destination flag should be overwritten
		_, ok := GetFlag[int, testUser](copiedCtx, "dst-flag")
		if ok {
			t.Error("Destination flag should have been overwritten")
		}
	})

	t.Run("NoSourceContext", func(t *testing.T) {
		srcCtx := context.Background()

		// Destination with existing context
		dstCtx := context.Background()
		dstCtx = WithUserID[int, testUser](dstCtx, 999)
		originalDstCtx := dstCtx

		copiedCtx := CopySRouterContextOverlay[int, testUser](dstCtx, srcCtx)

		if copiedCtx != originalDstCtx {
			t.Error("Expected destination context to be returned unchanged when source has no SRouterContext")
		}

		// Should still have original destination values
		userID, ok := GetUserID[int, testUser](copiedCtx)
		if !ok || userID != 999 {
			t.Errorf("Expected original destination UserID to be preserved. Got %d (ok: %v)", userID, ok)
		}
	})

	t.Run("DestinationHasNoContext_NoOp", func(t *testing.T) {
		srcCtx := createFullSRouterContext()
		dstCtx := context.Background()
		originalDstCtx := dstCtx

		copiedCtx := CopySRouterContextOverlay[int, testUser](dstCtx, srcCtx)

		if copiedCtx != originalDstCtx {
			t.Error("Expected destination context to be returned unchanged when destination has no SRouterContext")
		}

		// Should have no SRouterContext
		_, ok := GetSRouterContext[int, testUser](copiedCtx)
		if ok {
			t.Error("Expected no SRouterContext to be present in destination after no-op")
		}
	})

	t.Run("Independence", func(t *testing.T) {
		srcCtx := context.Background()
		srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", true)

		dstCtx := context.Background()
		dstCtx = WithUserID[int, testUser](dstCtx, 999) // Ensure destination has context

		copiedCtx := CopySRouterContextOverlay[int, testUser](dstCtx, srcCtx)

		// Modify source after copy
		srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", false)
		srcCtx = WithFlag[int, testUser](srcCtx, "source-only", true)

		// Modify copied after copy
		copiedCtx = WithFlag[int, testUser](copiedCtx, "copied-only", true)

		// Verify independence
		srcFlag, _ := GetFlag[int, testUser](srcCtx, "shared-flag")
		copiedFlag, _ := GetFlag[int, testUser](copiedCtx, "shared-flag")

		if srcFlag || !copiedFlag {
			t.Errorf("Contexts should be independent. Source flag: %v, Copied flag: %v", srcFlag, copiedFlag)
		}
	})
}

func TestCloneSRouterContext(t *testing.T) {
	t.Run("FullClone", func(t *testing.T) {
		srcCtx := createFullSRouterContext()
		srcRC, _ := GetSRouterContext[int, testUser](srcCtx)

		clonedRC := cloneSRouterContext(srcRC)

		// Verify clone has all values
		tempCtx := WithSRouterContext(context.Background(), clonedRC)
		verifyFullSRouterContext(t, tempCtx, "CloneSRouterContext")
	})

	t.Run("Independence", func(t *testing.T) {
		srcCtx := context.Background()
		srcCtx = WithFlag[int, testUser](srcCtx, "shared-flag", true)
		srcRC, _ := GetSRouterContext[int, testUser](srcCtx)

		clonedRC := cloneSRouterContext(srcRC)

		// Modify original
		srcRC.Flags["shared-flag"] = false
		srcRC.Flags["source-only"] = true

		// Modify clone
		clonedRC.Flags["copied-only"] = true

		// Verify independence
		if srcRC.Flags["shared-flag"] || !clonedRC.Flags["shared-flag"] {
			t.Error("Clone should be independent of source")
		}

		if _, exists := clonedRC.Flags["source-only"]; exists {
			t.Error("Source-only flag should not exist in clone")
		}

		if _, exists := srcRC.Flags["copied-only"]; exists {
			t.Error("Clone-only flag should not exist in source")
		}
	})

	t.Run("NilFlags", func(t *testing.T) {
		srcRC := &SRouterContext[int, testUser]{
			UserID:    123,
			UserIDSet: true,
			Flags:     nil,
		}

		clonedRC := cloneSRouterContext(srcRC)

		if clonedRC.Flags == nil {
			t.Error("Clone should have initialized Flags map even when source is nil")
		}

		if len(clonedRC.Flags) != 0 {
			t.Error("Clone should have empty Flags map when source is nil")
		}
	})

	t.Run("NilPathParams", func(t *testing.T) {
		srcRC := &SRouterContext[int, testUser]{
			UserID:     123,
			UserIDSet:  true,
			PathParams: nil,
			Flags:      make(map[string]bool),
		}

		clonedRC := cloneSRouterContext(srcRC)

		if clonedRC.PathParams != nil {
			t.Error("Clone should have nil PathParams when source is nil")
		}
	})
}
