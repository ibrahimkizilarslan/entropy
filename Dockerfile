# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install dependencies required for building
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the CLI application
RUN CGO_ENABLED=0 GOOS=linux go build -o entropy ./cmd/entropy

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install iproute2 for network chaos (tc)
RUN apk add --no-cache iproute2

# Copy the binary from builder
COPY --from=builder /app/entropy /usr/local/bin/entropy

# Set the entrypoint to the CLI
ENTRYPOINT ["entropy"]
