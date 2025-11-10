package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hoshichaam/pln_backend_go/internal/services"
)

type WalletHandler struct {
	svc *services.WalletService
}

func NewWalletHandler(s *services.WalletService) *WalletHandler {
	return &WalletHandler{svc: s}
}

func (h *WalletHandler) GetSaldo(c *fiber.Ctx) error {
	userID := c.Params("userId")
	out, err := h.svc.GetSaldo(c.Context(), userID)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(200).JSON(out)
}

func (h *WalletHandler) KlaimVoucher(c *fiber.Ctx) error {
	var req services.KlaimVoucherInput
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.svc.KlaimVoucher(c.Context(), req); err != nil {
		return mapError(c, err)
	}
	return c.Status(201).JSON(fiber.Map{"message": "klaim sukses"})
}

func (h *WalletHandler) TopUp(c *fiber.Ctx) error {
	var in services.TopUpInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	res, err := h.svc.TopUp(c.Context(), in)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(201).JSON(fiber.Map{"data": res})
}

func (h *WalletHandler) Withdraw(c *fiber.Ctx) error {
	var in services.WithdrawInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	res, err := h.svc.Withdraw(c.Context(), in)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(201).JSON(fiber.Map{"data": res})
}

func (h *WalletHandler) GetTransactions(c *fiber.Ctx) error {
	userID := c.Params("userId")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "userId wajib diisi"})
	}

	limit := c.QueryInt("limit", 50)
	items, err := h.svc.GetTransactions(c.Context(), userID, limit)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(200).JSON(fiber.Map{"data": items})
}

func (h *WalletHandler) GetPaymentStatus(c *fiber.Ctx) error {
	orderID := c.Params("orderId")
	if orderID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "orderId wajib diisi"})
	}
	res, err := h.svc.GetPaymentStatus(c.Context(), orderID)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(200).JSON(fiber.Map{"data": res})
}

func (h *WalletHandler) MidtransNotification(c *fiber.Ctx) error {
	if expected := h.svc.CallbackToken(); expected != "" {
		if token := c.Get("X-Callback-Token"); token != expected {
			return c.Status(403).JSON(fiber.Map{"error": "invalid callback token"})
		}
	}

	var payload services.NotificationPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.svc.HandleSnapNotification(c.Context(), payload); err != nil {
		return mapError(c, err)
	}
	return c.Status(200).JSON(fiber.Map{"status": "ok"})
}

func (h *WalletHandler) ListVouchers(c *fiber.Ctx) error {
	userID := c.Params("userId")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "userId wajib diisi"})
	}
	limit := c.QueryInt("limit", 50)
	items, err := h.svc.ListAvailableVouchers(c.Context(), userID, limit)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(200).JSON(fiber.Map{"data": items})
}

// mapper error
func mapError(c *fiber.Ctx, err error) error {
	switch err.(type) {
	case services.ErrBadRequest:
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	case services.ErrConflict:
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	case services.ErrInsufficientBalance:
		return c.Status(422).JSON(fiber.Map{"error": err.Error()})
	case services.ErrNotFoundResource:
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
}
