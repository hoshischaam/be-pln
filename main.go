package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"

	"github.com/hoshichaam/pln_backend_go/internal/database"
	"github.com/hoshichaam/pln_backend_go/internal/handlers"
	"github.com/hoshichaam/pln_backend_go/internal/middleware"
	"github.com/hoshichaam/pln_backend_go/internal/repositories"
	"github.com/hoshichaam/pln_backend_go/internal/services"
	myvalidator "github.com/hoshichaam/pln_backend_go/pkg/validator"
)

func main() {
	// 1) Load env (silent jika .env tidak ada)
	_ = godotenv.Load()

	// 2) Fail-fast kalau JWT_SECRET kosong
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		log.Fatal("JWT_SECRET tidak boleh kosong (set di .env)")
	}

	// 3) Connect DB (global database.DB)
	database.ConnectDB()
	defer func() {
		if database.DB != nil {
			_ = database.DB.Close()
		}
	}()

	// 4) Init dependencies
	v := myvalidator.New()
	repo := repositories.NewWalletRepo(database.DB)

	midtransServerKey := strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY"))
	if midtransServerKey == "" {
		log.Println("warning: MIDTRANS_SERVER_KEY kosong, integrasi Midtrans Snap dimatikan")
	}
	snapBaseURL := strings.TrimSpace(os.Getenv("MIDTRANS_SNAP_BASE_URL"))
	var snapClient *services.SnapClient
	if midtransServerKey != "" {
		snapClient = services.NewSnapClient(midtransServerKey, snapBaseURL)
	}

	irisClientKey := strings.TrimSpace(os.Getenv("MIDTRANS_IRIS_CLIENT_KEY"))
	irisClientSecret := strings.TrimSpace(os.Getenv("MIDTRANS_IRIS_CLIENT_SECRET"))
	irisBaseURL := strings.TrimSpace(os.Getenv("MIDTRANS_IRIS_BASE_URL"))
	if irisBaseURL == "" {
		irisBaseURL = "https://app.sandbox.midtrans.com/iris/api/v1/payouts"
	}
	var irisClient *services.IrisClient
	if irisClientKey != "" && irisClientSecret != "" {
		irisClient = services.NewIrisClient(irisClientKey, irisClientSecret, irisBaseURL)
	}

	callbackToken := strings.TrimSpace(os.Getenv("MIDTRANS_CALLBACK_TOKEN"))

	walletSvc := services.NewWalletService(repo, v, snapClient, irisClient, midtransServerKey, callbackToken)

	// 5) Init handlers
	walletHandler := handlers.NewWalletHandler(walletSvc)
	authHandler := handlers.NewAuthHandler(secret)

	// 6) Fiber app dengan timeout & proxy aware (untuk IP akurat di balik reverse proxy)
	app := fiber.New(fiber.Config{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,

		// ambil IP client di balik reverse proxy (nginx/cloudflare/traefik)
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		TrustedProxies: []string{
			"127.0.0.1", "::1", // local
			"10.0.0.0/8", // RFC1918
			"172.16.0.0/12",
			"192.168.0.0/16",
		},
		// (opsional) validasi format IP dari header
		EnableIPValidation: true,
	})

	// 7) CORS untuk cookie httpOnly (refresh token)
	// Set env: CORS_ORIGINS="http://localhost:5173,https://app.example.com"
	allowOrigins := os.Getenv("CORS_ORIGINS")
	if allowOrigins == "" {
		allowOrigins = "http://localhost:5173" // default dev
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowCredentials: true,
	}))

	// 8) Routes
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })

	api := app.Group("/api/v1")
	// wallet
	api.Get("/saldo/:userId", walletHandler.GetSaldo)
	api.Post("/klaim-voucher", walletHandler.KlaimVoucher)
	api.Post("/wallet/withdraw", walletHandler.Withdraw)
	api.Post("/tarik-saldo", walletHandler.Withdraw)
	api.Get("/wallet/transactions/:userId", walletHandler.GetTransactions)
	api.Get("/wallet/vouchers/:userId", walletHandler.ListVouchers)
	api.Post("/wallet/topup", walletHandler.TopUp)
	api.Post("/topup", walletHandler.TopUp)
	api.Get("/payment/status/:orderId", walletHandler.GetPaymentStatus)
	api.Post("/midtrans/notify", walletHandler.MidtransNotification)

	// auth
	api.Post("/auth/register", authHandler.Register)
	api.Post("/auth/login", authHandler.Login)
	api.Post("/auth/refresh", authHandler.Refresh)
	api.Post("/auth/logout", authHandler.Logout)
	api.Post("/auth/forgot-password", authHandler.ForgotPassword)
	api.Post("/auth/reset-password", middleware.JWTOptional(secret), authHandler.ResetPassword)

	// 9) Server start
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "3000"
	}
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting server on %s (CORS origins: %s)", addr, allowOrigins)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutdown signal received, stopping server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Server stopped gracefully.")
}
