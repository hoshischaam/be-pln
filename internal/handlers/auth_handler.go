// internal/handlers/auth_handler.go
package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/hoshichaam/pln_backend_go/internal/database"
	"github.com/hoshichaam/pln_backend_go/internal/models"
	"github.com/hoshichaam/pln_backend_go/pkg/authutil"
	response "github.com/hoshichaam/pln_backend_go/pkg/response"
	vld "github.com/hoshichaam/pln_backend_go/pkg/validator"
)

const (
	cookieRefresh = "refresh_token"
	cookieSID     = "sid"
)

type AuthHandler struct {
	jwtSecret string
}

func NewAuthHandler(secret string) *AuthHandler {
	return &AuthHandler{jwtSecret: secret}
}

// ------------------ helpers ------------------

func envDuration(key, def string) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		s = def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return mustParse(def)
	}
	return d
}

func mustParse(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

func envBcryptCost(def int) int {
	if v := os.Getenv("BCRYPT_COST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func isDev() bool { return strings.EqualFold(os.Getenv("APP_ENV"), "development") }

func allowResetTokenDebug() bool {
	if isDev() {
		return true
	}
	val := strings.TrimSpace(os.Getenv("EXPOSE_RESET_TOKENS"))
	fmt.Println("allowResetTokenDebug: EXPOSE_RESET_TOKENS =", val)
	return strings.EqualFold(val, "true") || val == "1"
}

// masking helper biar log aman
func maskEmail(e string) string {
	e = strings.TrimSpace(e)
	parts := strings.Split(e, "@")
	if len(parts) != 2 {
		if len(e) > 3 {
			return e[:3] + "***"
		}
		return "***"
	}
	local, domain := parts[0], parts[1]
	if len(local) > 2 {
		local = local[:2] + "***"
	} else {
		local = local + "***"
	}
	return local + "@" + domain
}

func shortToken(t string) string {
	t = strings.TrimSpace(t)
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}

// ------------------ handlers ------------------

// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	debugPrintln("AUTH Register: body parsed for", maskEmail(req.Email))

	if fields, err := vld.ValidateStruct(req); err != nil {
		debugPrintln("AUTH Register: validation failed", fields)
		return response.ValidationError(c, fields)
	}

	// cek email unik
	var exists bool
	if err := database.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, req.Email).Scan(&exists); err != nil {
		debugPrintln("AUTH Register: DB error on EXISTS:", err)
		return response.Error(c, fiber.StatusInternalServerError, "database error")
	}
	if exists {
		debugPrintln("AUTH Register: email already registered", maskEmail(req.Email))
		return response.Error(c, fiber.StatusConflict, "email already registered")
	}

	// hash password
	hash, err := authutil.HashPassword(req.Password, envBcryptCost(12))
	if err != nil {
		debugPrintln("AUTH Register: hash error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to hash password")
	}

	// simpan user + init wallet
	tx, err := database.DB.Begin()
	if err != nil {
		debugPrintln("AUTH Register: begin tx error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback()

	var userID string
	if err := tx.QueryRow(
		`INSERT INTO users (email, password_hash) VALUES ($1,$2) RETURNING id`,
		req.Email, hash,
	).Scan(&userID); err != nil {
		debugPrintln("AUTH Register: insert user error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to create user")
	}

	if _, err := tx.Exec(
		`INSERT INTO wallet_summary (user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`,
		userID,
	); err != nil {
		debugPrintln("AUTH Register: init wallet error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to init wallet")
	}

	if err := tx.Commit(); err != nil {
		debugPrintln("AUTH Register: commit error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to commit")
	}

	debugPrintln("AUTH Register: success userID=", userID, "email=", maskEmail(req.Email))
	return response.Created(c, fiber.Map{"userId": userID, "email": req.Email})
}

// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	debugPrintln("AUTH Login: body parsed for", maskEmail(req.Email))

	if fields, err := vld.ValidateStruct(req); err != nil {
		debugPrintln("AUTH Login: validation failed", fields)
		return response.ValidationError(c, fields)
	}

	// ambil user
	var userID, pwHash string
	err := database.DB.QueryRow(
		`SELECT id, password_hash FROM users WHERE email=$1`,
		req.Email,
	).Scan(&userID, &pwHash)
	if err == sql.ErrNoRows {
		debugPrintln("AUTH Login: user not found", maskEmail(req.Email))
		return response.Error(c, fiber.StatusUnauthorized, "email or password is incorrect")
	}
	if err != nil {
		debugPrintln("AUTH Login: DB error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "database error")
	}

	// cek password
	if bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(req.Password)) != nil {
		debugPrintln("AUTH Login: wrong password for", maskEmail(req.Email))
		return response.Error(c, fiber.StatusUnauthorized, "email or password is incorrect")
	}

	// buat access token (JWT)
	accessTTL := envDuration("ACCESS_TOKEN_TTL", "15m")
	at, err := authutil.NewAccessToken(h.jwtSecret, userID, accessTTL)
	if err != nil {
		debugPrintln("AUTH Login: issue access token error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to issue access token")
	}

	// buat refresh token (random raw) + simpan HASH ke DB
	raw := uuid.NewString()
	rHash, err := authutil.HashPassword(raw, bcrypt.MinCost)
	if err != nil {
		debugPrintln("AUTH Login: gagal hash refresh token:", err)
		return response.Error(c, fiber.StatusInternalServerError, "internal server error")
	}
	refreshExp := time.Now().Add(envDuration("REFRESH_TOKEN_TTL", "720h")) // 30d default

	// debugPrintln("DEBUG BUKTI:", "userID=", userID, "| rHash=", rHash, "| Panjang rHash=", len(rHash))

	var sid string
	if err := database.DB.QueryRow(
		`INSERT INTO sessions (user_id, refresh_token_hash, user_agent, ip_address, expires_at)
     VALUES ($1, $2, $3, $4, $5)
     RETURNING id`,
		userID, rHash, c.Get("User-Agent"), c.IP(), refreshExp,
	).Scan(&sid); err != nil {
		debugPrintln("AUTH Login: persist session error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to persist session")
	}

	// set cookies httpOnly (secure di production)
	c.Cookie(&fiber.Cookie{
		Name:     cookieRefresh,
		Value:    raw,
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   !isDev(), // true di production
		Expires:  refreshExp,
		Path:     "/",
	})
	c.Cookie(&fiber.Cookie{
		Name:     cookieSID,
		Value:    sid,
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   !isDev(),
		Expires:  refreshExp,
		Path:     "/",
	})

	debugPrintln("AUTH Login: success userID=", userID, "sid=", sid, "refresh=", shortToken(raw))
	return response.OK(c, models.AuthResponse{AccessToken: at, UserID: userID})
}

// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	sid := c.Cookies(cookieSID, "")
	raw := c.Cookies(cookieRefresh, "")
	if sid == "" || raw == "" {
		debugPrintln("AUTH Refresh: missing cookies sid/refresh")
		return response.Error(c, fiber.StatusUnauthorized, "no refresh token")
	}

	var userID, hash string
	err := database.DB.QueryRow(
		`SELECT user_id, refresh_token_hash
		   FROM sessions
		  WHERE id=$1 AND revoked_at IS NULL AND expires_at > now()`,
		sid,
	).Scan(&userID, &hash)
	if err != nil {
		debugPrintln("AUTH Refresh: session not found or DB error:", err, "sid=", sid)
		return response.Error(c, fiber.StatusUnauthorized, "invalid session")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(raw)) != nil {
		debugPrintln("AUTH Refresh: refresh hash mismatch sid=", sid)
		return response.Error(c, fiber.StatusUnauthorized, "invalid session")
	}

	// issue access token baru
	accessTTL := envDuration("ACCESS_TOKEN_TTL", "15m")
	at, err := authutil.NewAccessToken(h.jwtSecret, userID, accessTTL)
	if err != nil {
		debugPrintln("AUTH Refresh: issue access token error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to issue access token")
	}

	debugPrintln("AUTH Refresh: success userID=", userID, "sid=", sid)
	return response.OK(c, models.AuthResponse{AccessToken: at, UserID: userID})
}

// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	sid := c.Cookies(cookieSID, "")
	raw := c.Cookies(cookieRefresh, "")

	if sid != "" && raw != "" {
		var hash string
		err := database.DB.QueryRow(
			`SELECT refresh_token_hash
			   FROM sessions
			  WHERE id=$1 AND revoked_at IS NULL AND expires_at > now()`,
			sid,
		).Scan(&hash)
		if err == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(raw)) == nil {
			if _, err := database.DB.Exec(`UPDATE sessions SET revoked_at=now() WHERE id=$1`, sid); err != nil {
				debugPrintln("AUTH Logout: revoke error:", err, "sid=", sid)
			} else {
				debugPrintln("AUTH Logout: revoked sid=", sid)
			}
		} else {
			debugPrintln("AUTH Logout: session check failed sid=", sid, "err=", err)
		}
	} else {
		debugPrintln("AUTH Logout: missing cookies")
	}

	// hapus cookies
	c.Cookie(&fiber.Cookie{Name: cookieRefresh, Value: "", Expires: time.Unix(0, 0), HTTPOnly: true, Path: "/"})
	c.Cookie(&fiber.Cookie{Name: cookieSID, Value: "", Expires: time.Unix(0, 0), HTTPOnly: true, Path: "/"})
	return response.NoContent(c)
}

// POST /api/v1/auth/forgot-password
func (h *AuthHandler) ForgotPassword(c *fiber.Ctx) error {
	var req models.ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	debugPrintln("AUTH Forgot: body parsed for", maskEmail(req.Email))

	if fields, err := vld.ValidateStruct(req); err != nil {
		debugPrintln("AUTH Forgot: validation failed", fields)
		return response.ValidationError(c, fields)
	}

	var userID string
	if err := database.DB.QueryRow(`SELECT id FROM users WHERE email=$1`, req.Email).Scan(&userID); err != nil {
		// jawaban generik → tidak membocorkan apakah email terdaftar
		debugPrintln("AUTH Forgot: user not found (masked)", maskEmail(req.Email))
		return response.OK(c, fiber.Map{"message": "If the email exists, a reset link has been sent"})
	}

	// token raw → hash ke DB
	raw := strings.ReplaceAll(uuid.NewString(), "-", "") + "." + strings.ReplaceAll(uuid.NewString(), "-", "")
	hash, _ := authutil.HashPassword(raw, bcrypt.MinCost)
	exp := time.Now().Add(1 * time.Hour)

	var tokenID string
	if err := database.DB.QueryRow(
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		 VALUES ($1,$2,$3) RETURNING id`,
		userID, hash, exp,
	).Scan(&tokenID); err != nil {
		debugPrintln("AUTH Forgot: create token error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to create reset token")
	}

	// TODO: kirim email berisi link reset (tokenID + raw token via URL) ke user
	out := fiber.Map{"message": "If the email exists, a reset link has been sent"}
	if allowResetTokenDebug() {
		// untuk dev/testing manual
		out["tokenId"] = tokenID
		out["tokenDev"] = raw
	}
	debugPrintln("AUTH Forgot: token issued userID=", userID, "tokenId=", tokenID, "token=", shortToken(raw))
	return response.OK(c, out)
}

// POST /api/v1/auth/reset-password
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req models.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body")
	}

	tx, err := database.DB.Begin()
	if err != nil {
		debugPrintln("AUTH Reset: begin tx error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to start transaction")
	}
	defer tx.Rollback()

	var targetUserID string

	// Mode 1: reset via token (lupa password)
	newPassword := strings.TrimSpace(req.NewPassword)
	if newPassword == "" {
		newPassword = strings.TrimSpace(req.Password)
	}
	if len(newPassword) < 8 {
		return response.Error(c, fiber.StatusBadRequest, "password baru minimal 8 karakter")
	}

	req.Token = strings.TrimSpace(req.Token)
	req.TokenID = strings.TrimSpace(req.TokenID)
	if req.TokenID != "" && req.Token != "" {
		debugPrintln("AUTH Reset via token: tokenId=", req.TokenID)

		var tokenHash string
		var expiresAt, usedAt sql.NullTime
		err := database.DB.QueryRow(
			`SELECT user_id, token_hash, expires_at, used_at
			   FROM password_reset_tokens
			  WHERE id=$1`,
			req.TokenID,
		).Scan(&targetUserID, &tokenHash, &expiresAt, &usedAt)
		if err != nil || usedAt.Valid || !expiresAt.Valid || time.Now().After(expiresAt.Time) {
			debugPrintln("AUTH Reset: invalid/expired tokenId=", req.TokenID, "err=", err)
			return response.Error(c, fiber.StatusBadRequest, "token invalid or expired")
		}

		if bcrypt.CompareHashAndPassword([]byte(tokenHash), []byte(req.Token)) != nil {
			debugPrintln("AUTH Reset: token mismatch tokenId=", req.TokenID)
			return response.Error(c, fiber.StatusBadRequest, "token invalid")
		}

		if _, err := tx.Exec(`UPDATE password_reset_tokens SET used_at=now() WHERE id=$1`, req.TokenID); err != nil {
			debugPrintln("AUTH Reset: close token error:", err)
			return response.Error(c, fiber.StatusInternalServerError, "failed to close token")
		}
	} else {
		// Mode 2: ganti password dari aplikasi (perlu JWT + oldPassword)
		if strings.TrimSpace(req.OldPassword) == "" {
			return response.Error(c, fiber.StatusBadRequest, "oldPassword wajib diisi")
		}
		if userID, ok := c.Locals("userId").(string); ok && strings.TrimSpace(userID) != "" {
			targetUserID = strings.TrimSpace(userID)
		} else {
			return response.Error(c, fiber.StatusUnauthorized, "akses memerlukan autentikasi")
		}
		var currentHash string
		if err := database.DB.QueryRow(`SELECT password_hash FROM users WHERE id=$1`, targetUserID).Scan(&currentHash); err != nil {
			return response.Error(c, fiber.StatusInternalServerError, "failed to fetch user")
		}
		if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)) != nil {
			return response.Error(c, fiber.StatusBadRequest, "old password is incorrect")
		}
	}

	newHash, err := authutil.HashPassword(newPassword, envBcryptCost(12))
	if err != nil {
		debugPrintln("AUTH Reset: hash new password error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to hash password")
	}

	if _, err := tx.Exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, newHash, targetUserID); err != nil {
		debugPrintln("AUTH Reset: update users error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to update password")
	}

	if err := tx.Commit(); err != nil {
		debugPrintln("AUTH Reset: commit error:", err)
		return response.Error(c, fiber.StatusInternalServerError, "failed to commit")
	}

	debugPrintln("AUTH Reset: success userID=", targetUserID, "tokenId=", req.TokenID)
	return response.OK(c, fiber.Map{"message": "password has been reset"})
}

// ------------------ debug (dipakai beneran di atas) ------------------
func debugPrintln(a ...any) {
	if os.Getenv("APP_ENV") == "development" {
		fmt.Println(a...)
	}
}
