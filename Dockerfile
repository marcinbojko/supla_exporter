# Start from the official Golang image
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o supla_exporter .

# Start a new stage from scratch
FROM alpine:3.21


# Create non-root user first
RUN addgroup -S supla && \
    adduser -S supla_exporter -G supla

# Install ca-certificates for HTTPS requests
# hadolint global ignore=DL3018
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/supla_exporter .

# Set ownership of the application files
RUN chown -R supla_exporter:supla /app
USER supla_exporter

HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:2112/metrics || exit 1

# Expose the port the app runs on
EXPOSE 2112
# Add  labels
LABEL release="supla_exporter"
LABEL maintainer="marcinbojko"
# Command to run the executable
CMD ["./supla_exporter"]
