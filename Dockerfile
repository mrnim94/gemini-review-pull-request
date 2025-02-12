# Dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy and build the Go application
COPY . .
RUN go build -o /action main.go

# Use a minimal base image
FROM alpine:latest
COPY --from=builder /action /action

# Set the entrypoint
ENTRYPOINT ["/action"]
