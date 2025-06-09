# Transaction Management Example

This example demonstrates SRouter's automatic transaction management feature using a simple banking application with user accounts and money transfers.

## Features Demonstrated

1. **Automatic Transaction Management**: Routes automatically run within database transactions
2. **Commit on Success**: Transactions are committed when handlers return successfully
3. **Rollback on Error**: Transactions are rolled back on errors or invalid status codes
4. **Transaction Configuration**: Per-route transaction settings
5. **Real-world Use Case**: Money transfer between accounts with ACID guarantees

## Running the Example

1. Install dependencies:
```bash
go mod download
```

2. Run the application:
```bash
go run main.go
```

The server will start on port 8080 with an SQLite database.

## API Endpoints

### 1. Create User (Transactional)
Creates a new user and their account within a transaction.

```bash
# Success case
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"John Doe","email":"john@example.com"}'

# Failure case (duplicate email) - will rollback
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Another User","email":"alice@example.com"}'

# Business rule failure - will rollback
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Failed User","email":"fail@example.com"}'
```

### 2. Transfer Money (Transactional)
Transfers money between accounts with full ACID guarantees.

```bash
# Success case
curl -X POST http://localhost:8080/api/transfer \
  -H "Content-Type: application/json" \
  -d '{"from_user_id":1,"to_user_id":2,"amount":100}'

# Failure case (insufficient balance) - will rollback
curl -X POST http://localhost:8080/api/transfer \
  -H "Content-Type: application/json" \
  -d '{"from_user_id":1,"to_user_id":2,"amount":10000}'
```

### 3. Health Check (Non-transactional)
Simple health check endpoint that explicitly disables transactions.

```bash
curl http://localhost:8080/api/health
```

## How It Works

1. **Transaction Factory**: The `GormTransactionFactory` creates new database transactions
2. **Automatic Management**: SRouter automatically:
   - Begins a transaction before the handler
   - Adds it to the request context
   - Commits on success (2xx/3xx status)
   - Rolls back on error (4xx/5xx status or handler error)
3. **Handler Access**: Handlers retrieve the transaction using `scontext.GetTransactionFromRequest`
4. **Database Operations**: All operations use the transaction's database connection

## Key Points

- Transactions are only created for routes with `Transaction.Enabled = true`
- The entire handler runs within a single transaction
- Multiple database operations are atomic
- Rollback happens automatically on any error
- No manual transaction management code needed

## Testing Transaction Behavior

1. **Test Rollback on Duplicate Email**:
   - Try creating a user with an existing email
   - Check that no new user or account was created

2. **Test Rollback on Business Rule**:
   - Try creating a user with email "fail@example.com"
   - Verify that neither user nor account was created

3. **Test Money Transfer**:
   - Transfer money between accounts
   - Check balances are updated atomically
   - Try transferring more than available balance
   - Verify no partial updates occurred

## Database Schema

The example uses SQLite with two tables:

```sql
-- Users table
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT,
    email TEXT UNIQUE,
    created_at TIMESTAMP
);

-- Accounts table
CREATE TABLE accounts (
    id INTEGER PRIMARY KEY,
    user_id INTEGER,
    balance REAL
);
```

## Extending the Example

You can extend this example by:
1. Adding more complex business logic
2. Using transaction savepoints for partial rollbacks
3. Implementing read-only transactions for queries
4. Adding transaction timeout handling
5. Using different isolation levels