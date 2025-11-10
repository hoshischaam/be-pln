package response

import "github.com/gofiber/fiber/v2"

// Envelope standar biar konsisten
type Envelope map[string]any

// ---- Sukses ----
func OK(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(Envelope{"data": data})
}

func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(Envelope{"data": data})
}

func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// ---- Error umum ----
type APIError struct {
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
}

func Error(c *fiber.Ctx, code int, msg string) error {
	return c.Status(code).JSON(Envelope{"error": APIError{Message: msg}})
}

// ---- Error validasi (field-based) ----
func ValidationError(c *fiber.Ctx, fields map[string]string) error {
	return c.Status(fiber.StatusBadRequest).JSON(Envelope{
		"error": APIError{
			Message: "validation failed",
			Detail:  map[string]any{"fields": fields},
		},
	})
}
