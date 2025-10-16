# Multi-stage build for Neorg Documentation Lambda

# Stage 1: Build Go application
FROM golang:1.21-alpine AS go-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY serverless/ ./serverless/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o neorg-lambda ./serverless

# Stage 2: Runtime environment with Neovim
FROM alpine:3.19

# Install system dependencies
RUN apk add --no-cache \
    neovim \
    git \
    luarocks \
    wget \
    curl \
    nodejs \
    npm \
    unzip \
    ca-certificates \
    && rm -rf /var/cache/apk/*

# Create app user
RUN addgroup -g 1001 -S appuser && \
    adduser -S appuser -u 1001 -G appuser

# Create necessary directories
RUN mkdir -p /app/.config/nvim /app/data /tmp/workdir
WORKDIR /tmp/workdir

# Copy Neovim configuration
COPY .config/nvim/init.lua /app/.config/nvim/

# Copy the built Go binary
COPY --from=go-builder /app/neorg-lambda /app/

# Set ownership
RUN chown -R appuser:appuser /app /tmp/workdir

# Switch to app user
USER appuser

# Set environment variables for Neovim
ENV XDG_CONFIG_HOME=/app/.config
ENV XDG_DATA_HOME=/app/data
ENV XDG_CACHE_HOME=/app/cache

# Pre-install Neovim plugins in headless mode
RUN nvim --headless "+Lazy! sync" +qa || true

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["/app/neorg-lambda"]