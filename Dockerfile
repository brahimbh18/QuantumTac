# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# Runtime stage
FROM alpine:3.21

RUN apk --no-cache add ca-certificates

COPY --from=builder /server /server

EXPOSE 8081

ENTRYPOINT ["/server"]
