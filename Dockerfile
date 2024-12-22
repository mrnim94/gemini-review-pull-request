# Use the official Golang image to build the application
FROM golang:1.20 as builder

# Set the working directory
WORKDIR /app

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN go build -o action .

# Use a minimal image for the final build
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the binary from the builder
COPY --from=builder /app/action .

# Entry point
ENTRYPOINT ["./action"]
