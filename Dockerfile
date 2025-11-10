FROM golang:1.24-alpine AS build
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest


WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server main.go

FROM alpine:latest
COPY --from=build /go/bin/migrate /usr/local/bin/
COPY --from=build /server /server
COPY --from=build /app/migrations ./migrations

EXPOSE 3000

ENTRYPOINT ["/server"]