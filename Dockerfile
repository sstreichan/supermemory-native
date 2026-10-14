# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy dependency files and download
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build clean static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o supermemory-native ./cmd/supermemory-native

# Run stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates in case they need to connect to cloud embedding APIs (like Gemini)
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/supermemory-native /app/supermemory-native

# Default port; bind all container interfaces so Docker port mapping works
EXPOSE 6767
ENV HOST=0.0.0.0

# Run binary
ENTRYPOINT ["/app/supermemory-native"]
