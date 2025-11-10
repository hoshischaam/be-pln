package validator

import (
	"sync"

	v10 "github.com/go-playground/validator/v10"
)

// Singleton validator dari go-playground
var (
	once sync.Once
	v    *v10.Validate
)

// New mengembalikan instance validator yang sama (thread-safe).
func New() *v10.Validate {
	once.Do(func() {
		v = v10.New()
		// Di sini bisa tambahkan custom tag/validation kalau perlu
	})
	return v
}

// ValidateStruct memvalidasi struct dan merapikan error menjadi map[field]message.
func ValidateStruct(s any) (map[string]string, error) {
	err := New().Struct(s)
	if err == nil {
		return nil, nil
	}
	ve, ok := err.(v10.ValidationErrors)
	if !ok {
		// bukan error validasi terstruktur
		return map[string]string{"_": err.Error()}, err
	}
	fields := make(map[string]string, len(ve))
	for _, fe := range ve {
		// contoh pesan simple; bisa kamu kustom sesuai kebutuhan
		fields[fe.Field()] = msgForTag(fe)
	}
	return fields, err
}

// msgForTag bikin pesan ringkas per rule
func msgForTag(fe v10.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "field is required"
	case "email":
		return "must be a valid email"
	case "min":
		return "too short"
	case "max":
		return "too long"
	case "uuid4":
		return "must be a valid UUIDv4"
	default:
		return fe.Error() // fallback detail bawaan
	}
}
