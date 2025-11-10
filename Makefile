MIGRATE := go run ./cmd/migrate/main.go

.PHONY: migrate-up
migrate-up:
	@$(MIGRATE) -cmd up

.PHONY: migrate-down
migrate-down:
	@$(MIGRATE) -cmd down

.PHONY: migrate-version
migrate-version:
	@$(MIGRATE) -cmd version

.PHONY: migrate-steps
migrate-steps:
	@if [ -z "$(steps)" ]; then \
		echo "Usage: make migrate-steps steps=<n>"; \
		exit 1; \
	fi
	@$(MIGRATE) -cmd steps -steps $(steps)

.PHONY: migrate-force
migrate-force:
	@if [ -z "$(version)" ]; then \
		echo "Usage: make migrate-force version=<n>"; \
		exit 1; \
	fi
	@$(MIGRATE) -cmd force -forceVersion $(version)
