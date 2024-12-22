# Build stage
FROM golang:1.23 as builder

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the Go application
RUN go build -o /action .

# Final stage
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder
COPY --from=builder /action .

# Set the entry point to the Go binary
ENTRYPOINT ["./action"]