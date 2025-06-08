package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	Name      string    `json:"name"`
	Email     string    `json:"email" gorm:"unique"`
	CreatedAt time.Time `json:"created_at"`
}

// Request/Response types
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserResponse struct {
	User    User   `json:"user"`
	Message string `json:"message"`
}

type TransferRequest struct {
	FromUserID uint    `json:"from_user_id"`
	ToUserID   uint    `json:"to_user_id"`
	Amount     float64 `json:"amount"`
}

type TransferResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Account model for demonstrating transactions
type Account struct {
	ID      uint    `json:"id" gorm:"primarykey"`
	UserID  uint    `json:"user_id"`
	Balance float64 `json:"balance"`
}

// GormTransactionFactory implements common.TransactionFactory
type GormTransactionFactory struct {
	db *gorm.DB
}

func (f *GormTransactionFactory) BeginTransaction(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
	// Begin transaction with context
	tx := f.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	
	// Wrap with GormTransactionWrapper
	return middleware.NewGormTransactionWrapper(tx), nil
}

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize database
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect database:", err)
	}

	// Migrate the schema
	db.AutoMigrate(&User{}, &Account{})

	// Create transaction factory
	txFactory := &GormTransactionFactory{db: db}

	// Create router with transaction support
	r := router.NewRouter[string, User](router.RouterConfig{
		Logger:             logger,
		TransactionFactory: txFactory,
		// Enable transactions globally (can be overridden per route)
		GlobalTransaction: &common.TransactionConfig{
			Enabled: true,
		},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []router.RouteDefinition{
					// Create user with transaction (will rollback on error)
					router.NewGenericRouteDefinition[CreateUserRequest, CreateUserResponse, string, User](
						router.RouteConfig[CreateUserRequest, CreateUserResponse]{
							Path:    "/users",
							Methods: []router.HttpMethod{router.MethodPost},
							Codec:   codec.NewJSONCodec[CreateUserRequest, CreateUserResponse](),
							Handler: createUserHandler(db),
						},
					),
					// Transfer money between accounts (classic transaction use case)
					router.NewGenericRouteDefinition[TransferRequest, TransferResponse, string, User](
						router.RouteConfig[TransferRequest, TransferResponse]{
							Path:    "/transfer",
							Methods: []router.HttpMethod{router.MethodPost},
							Codec:   codec.NewJSONCodec[TransferRequest, TransferResponse](),
							Handler: transferHandler,
						},
					),
					// Health check without transaction
					router.RouteConfigBase{
						Path:    "/health",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: healthHandler,
						Overrides: common.RouteOverrides{
							Transaction: &common.TransactionConfig{
								Enabled: false, // Disable transaction for health check
							},
						},
					},
				},
			},
		},
	}, nil, nil)

	// Seed some test data
	seedTestData(db)

	fmt.Println("Server starting on :8080")
	fmt.Println("\nExample requests:")
	fmt.Println("1. Create user (with transaction):")
	fmt.Println(`   curl -X POST http://localhost:8080/api/users -H "Content-Type: application/json" -d '{"name":"John Doe","email":"john@example.com"}'`)
	fmt.Println("\n2. Transfer money (with transaction):")
	fmt.Println(`   curl -X POST http://localhost:8080/api/transfer -H "Content-Type: application/json" -d '{"from_user_id":1,"to_user_id":2,"amount":50}'`)
	fmt.Println("\n3. Health check (no transaction):")
	fmt.Println(`   curl http://localhost:8080/api/health`)

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal("Server failed:", err)
	}
}

// createUserHandler demonstrates transaction usage with potential rollback
func createUserHandler(mainDB *gorm.DB) router.GenericHandler[CreateUserRequest, CreateUserResponse] {
	return func(r *http.Request, req CreateUserRequest) (CreateUserResponse, error) {
		// Get transaction from context
		tx, ok := scontext.GetTransactionFromRequest[string, User](r)
		if !ok {
			return CreateUserResponse{}, fmt.Errorf("no transaction available")
		}

		// Get the GORM database handle from transaction
		db := tx.GetDB()

		// Create user
		user := User{
			Name:      req.Name,
			Email:     req.Email,
			CreatedAt: time.Now(),
		}

		if err := db.Create(&user).Error; err != nil {
			// Returning error will cause automatic rollback
			return CreateUserResponse{}, fmt.Errorf("failed to create user: %w", err)
		}

		// Create account for user
		account := Account{
			UserID:  user.ID,
			Balance: 100.0, // Starting balance
		}

		if err := db.Create(&account).Error; err != nil {
			// This will also cause rollback, undoing the user creation
			return CreateUserResponse{}, fmt.Errorf("failed to create account: %w", err)
		}

		// Simulate a business rule check that might fail
		if user.Email == "fail@example.com" {
			// This will rollback both user and account creation
			return CreateUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "This email is not allowed")
		}

		// Success - transaction will be committed automatically
		return CreateUserResponse{
			User:    user,
			Message: "User and account created successfully",
		}, nil
	}
}

// transferHandler demonstrates a classic transaction use case
func transferHandler(r *http.Request, req TransferRequest) (TransferResponse, error) {
	// Get transaction from context
	tx, ok := scontext.GetTransactionFromRequest[string, User](r)
	if !ok {
		return TransferResponse{}, fmt.Errorf("no transaction available")
	}

	db := tx.GetDB()

	// Lock accounts for update (prevents race conditions)
	var fromAccount, toAccount Account
	
	if err := db.Set("gorm:query_option", "FOR UPDATE").First(&fromAccount, "user_id = ?", req.FromUserID).Error; err != nil {
		return TransferResponse{}, fmt.Errorf("sender account not found")
	}

	if err := db.Set("gorm:query_option", "FOR UPDATE").First(&toAccount, "user_id = ?", req.ToUserID).Error; err != nil {
		return TransferResponse{}, fmt.Errorf("recipient account not found")
	}

	// Check sufficient balance
	if fromAccount.Balance < req.Amount {
		// This will cause a rollback
		return TransferResponse{}, router.NewHTTPError(http.StatusBadRequest, "Insufficient balance")
	}

	// Perform transfer
	fromAccount.Balance -= req.Amount
	toAccount.Balance += req.Amount

	if err := db.Save(&fromAccount).Error; err != nil {
		return TransferResponse{}, fmt.Errorf("failed to update sender balance")
	}

	if err := db.Save(&toAccount).Error; err != nil {
		// This would rollback the previous update too
		return TransferResponse{}, fmt.Errorf("failed to update recipient balance")
	}

	// Success - transaction will be committed
	return TransferResponse{
		Success: true,
		Message: fmt.Sprintf("Transferred %.2f from user %d to user %d", req.Amount, req.FromUserID, req.ToUserID),
	}, nil
}

// healthHandler is a simple handler without transaction
func healthHandler(w http.ResponseWriter, r *http.Request) {
	// This handler runs without a transaction due to route override
	response := map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// seedTestData creates initial test data
func seedTestData(db *gorm.DB) {
	// Clear existing data
	db.Exec("DELETE FROM accounts")
	db.Exec("DELETE FROM users")

	// Create test users and accounts
	users := []User{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
	}

	for _, user := range users {
		db.Create(&user)
		db.Create(&Account{
			UserID:  user.ID,
			Balance: 1000.0,
		})
	}

	fmt.Println("Test data seeded: Created 2 users with accounts (balance: 1000.0 each)")
}