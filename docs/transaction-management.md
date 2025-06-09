# Transaction Management

SRouter provides automatic transaction management for database operations, allowing you to declaratively specify which routes should run within a database transaction. When enabled, the framework automatically begins a transaction before executing the handler and commits or rolls back based on the handler's success.

## Overview

Transaction management in SRouter:
- Automatically begins transactions before handler execution
- Commits on successful responses (2xx and 3xx status codes)
- Rolls back on errors (4xx and 5xx status codes) or panics
- Works with any database that implements the `DatabaseTransaction` interface
- Follows the standard configuration hierarchy (route > subrouter > global)

## Configuration

### 1. Implement TransactionFactory

First, implement the `TransactionFactory` interface for your database:

```go
type MyTransactionFactory struct {
    db *gorm.DB
}

func (f *MyTransactionFactory) BeginTransaction(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
    // Extract options if needed
    var txOptions *sql.TxOptions
    if isolation, ok := options["isolation"].(sql.IsolationLevel); ok {
        txOptions = &sql.TxOptions{
            Isolation: isolation,
        }
    }
    
    // Begin transaction
    tx := f.db.WithContext(ctx).Begin(txOptions)
    if tx.Error != nil {
        return nil, tx.Error
    }
    
    // Wrap with GormTransactionWrapper
    return middleware.NewGormTransactionWrapper(tx), nil
}
```

### 2. Configure the Router

Add the transaction factory to your router configuration:

```go
router := router.NewRouter[string, User](router.RouterConfig{
    Logger: logger,
    TransactionFactory: &MyTransactionFactory{db: db},
    // Global transaction configuration (optional)
    GlobalTransaction: &common.TransactionConfig{
        Enabled: true,
        Options: map[string]any{
            "isolation": sql.LevelReadCommitted,
        },
    },
}, authFunc, userIDFunc)
```

### 3. Enable Transactions for Routes

#### For Individual Routes

```go
router.RegisterRoute(router.RouteConfigBase{
    Path:    "/users",
    Methods: []router.HttpMethod{router.MethodPost},
    Handler: createUserHandler,
    Overrides: common.RouteOverrides{
        Transaction: &common.TransactionConfig{
            Enabled: true,
        },
    },
})
```

#### For Generic Routes

```go
router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, User](
    router.RouteConfig[CreateUserReq, CreateUserResp]{
        Path:    "/users",
        Methods: []router.HttpMethod{router.MethodPost},
        Codec:   codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
        Handler: createUserHandler,
        Overrides: common.RouteOverrides{
            Transaction: &common.TransactionConfig{
                Enabled: true,
                Options: map[string]any{
                    "isolation": sql.LevelSerializable,
                },
            },
        },
    },
)
```

#### For Subrouters

```go
router.RouterConfig{
    SubRouters: []router.SubRouterConfig{
        {
            PathPrefix: "/api",
            Overrides: common.RouteOverrides{
                Transaction: &common.TransactionConfig{
                    Enabled: true,
                },
            },
            Routes: []router.RouteDefinition{
                // All routes here will have transactions enabled by default
            },
        },
    },
}
```

## Using Transactions in Handlers

Access the transaction from the request context:

```go
func createUserHandler(w http.ResponseWriter, r *http.Request) {
    // Get the transaction
    tx, ok := scontext.GetTransactionFromRequest[string, User](r)
    if !ok {
        http.Error(w, "No transaction available", http.StatusInternalServerError)
        return
    }
    
    // Get the underlying database connection (for GORM)
    db := tx.GetDB()
    
    // Perform database operations
    var user User
    if err := db.Create(&user).Error; err != nil {
        // Return error response - transaction will be rolled back
        http.Error(w, "Failed to create user", http.StatusInternalServerError)
        return
    }
    
    // Return success - transaction will be committed
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(user)
}
```

## Transaction Behavior

### Automatic Commit

Transactions are automatically committed when:
- Handler returns without error (for generic routes)
- Response status is 2xx or 3xx (for all routes)
- No panic occurs

### Automatic Rollback

Transactions are automatically rolled back when:
- Handler returns an error (for generic routes)
- Response status is 4xx or 5xx
- A panic occurs (caught by recovery middleware)
- Transaction factory fails to create a transaction

### Configuration Hierarchy

Transaction configuration follows the standard SRouter hierarchy:
1. Route-specific configuration (highest priority)
2. Subrouter configuration
3. Global router configuration (lowest priority)

Example:
```go
// Global: transactions disabled
GlobalTransaction: nil,

SubRouters: []SubRouterConfig{
    {
        PathPrefix: "/api",
        Overrides: RouteOverrides{
            // Subrouter: transactions enabled for all /api routes
            Transaction: &TransactionConfig{Enabled: true},
        },
        Routes: []RouteDefinition{
            RouteConfigBase{
                Path: "/health",
                // Route: transactions disabled for this specific route
                Overrides: RouteOverrides{
                    Transaction: &TransactionConfig{Enabled: false},
                },
            },
        },
    },
}
```

## Advanced Usage

### Custom Transaction Options

Pass database-specific options through the configuration:

```go
Transaction: &common.TransactionConfig{
    Enabled: true,
    Options: map[string]any{
        "isolation": sql.LevelSerializable,
        "read_only": true,
        "timeout": 30 * time.Second,
    },
}
```

### Savepoints

Use savepoints for nested transaction-like behavior:

```go
tx, _ := scontext.GetTransactionFromRequest[string, User](r)

// Create a savepoint
if err := tx.SavePoint("before_risky_operation"); err != nil {
    // Handle error
}

// Perform risky operation
if err := riskyOperation(tx.GetDB()); err != nil {
    // Rollback to savepoint
    if err := tx.RollbackTo("before_risky_operation"); err != nil {
        // Handle rollback error
    }
    // Continue with alternative logic
} else {
    // Operation succeeded, continue
}
```

### Testing with Transactions

Use the mock transaction factory for testing:

```go
import "github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"

mockFactory := &mocks.MockTransactionFactory{
    BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
        return &mocks.MockTransaction{
            CommitFunc: func() error {
                // Track commits in tests
                return nil
            },
        }, nil
    },
}

router := router.NewRouter[string, User](router.RouterConfig{
    TransactionFactory: mockFactory,
}, authFunc, userIDFunc)
```

## Best Practices

1. **Idempotency**: Design handlers to be idempotent when possible, as transactions may be retried
2. **Timeout Handling**: Set appropriate timeouts for long-running transactions
3. **Error Responses**: Return appropriate HTTP status codes to trigger correct commit/rollback behavior
4. **Connection Pooling**: Ensure your transaction factory properly manages database connections
5. **Isolation Levels**: Choose appropriate isolation levels based on your consistency requirements

## Performance Considerations

- Transactions are only created when explicitly enabled - no overhead for non-transactional routes
- The framework adds minimal overhead beyond the database transaction itself
- Consider connection pool limits when enabling transactions globally
- Use read-only transactions when appropriate for better performance

## Compatibility

The transaction management system works with any database that can implement the `DatabaseTransaction` interface. A GORM adapter (`GormTransactionWrapper`) is provided out of the box, but you can implement the interface for any database:

```go
type DatabaseTransaction interface {
    Commit() error
    Rollback() error
    SavePoint(name string) error
    RollbackTo(name string) error
    GetDB() *gorm.DB  // Or your database type
}
```