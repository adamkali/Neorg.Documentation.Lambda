# Multi-stage build for Neorg Documentation Lambda

# Stage 1: Build Go application
FROM golang:1.25 AS go-builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY serverless/ ./serverless/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o neorg-lambda ./serverless

# Stage 2: Runtime environment with Neovim
FROM ubuntu:22.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    wget \
    curl \
    nodejs \
    npm \
    unzip \
    ca-certificates \
    locales \
    libc6 \
    libgcc-s1 \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/* \
    && locale-gen en_US.UTF-8

# Set locale environment variables
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US:en \
    LC_ALL=en_US.UTF-8

# Download and install Neovim 0.10+ from official tarball
RUN wget https://github.com/neovim/neovim/releases/latest/download/nvim-linux-x86_64.tar.gz \
    && tar xzvf nvim-linux-x86_64.tar.gz \
    && mv nvim-linux-x86_64 /opt/nvim \
    && ln -sf /opt/nvim/bin/nvim /usr/local/bin/nvim \
    && rm nvim-linux-x86_64.tar.gz

# Create app user
RUN groupadd -g 1001 appuser && \
    useradd -r -u 1001 -g appuser -m appuser

# Create necessary directories
RUN mkdir -p /app/.config/nvim /app/data /tmp/workdir /home/appuser
WORKDIR /tmp/workdir

# Copy Neovim configuration
COPY .config/nvim/init.lua /app/.config/nvim/

# Copy the built Go binary
COPY --from=go-builder /app/neorg-lambda /app/

# Set permissions and ownership
RUN chmod +x /app/neorg-lambda && chown -R appuser:appuser /app /tmp/workdir /opt/nvim /home/appuser

# Switch to app user
USER appuser

# Set environment variables for Neovim
ENV XDG_CONFIG_HOME=/app/.config
ENV XDG_DATA_HOME=/app/data
ENV XDG_CACHE_HOME=/app/cache

# Pre-install Neovim plugins in headless mode
RUN /opt/nvim/bin/nvim --headless "+Lazy! sync" +qa || true

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["/app/neorg-lambda"]