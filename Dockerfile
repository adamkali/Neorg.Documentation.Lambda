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

# Install minimal system dependencies including C compiler for TreeSitter
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    wget \
    curl \
    unzip \
    make \
    ca-certificates \
    lua5.1 \
    liblua5.1-0 \
    gcc \
    g++ \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Set locale environment variables
ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

# Download and install Neovim 0.10+ from official tarball
RUN wget https://github.com/neovim/neovim/releases/latest/download/nvim-linux-x86_64.tar.gz \
    && tar xzvf nvim-linux-x86_64.tar.gz \
    && mv nvim-linux-x86_64 /opt/nvim \
    && ln -sf /opt/nvim/bin/nvim /usr/local/bin/nvim \
    && rm nvim-linux-x86_64.tar.gz

# Since lua-utils might not be available via luarocks, let's try a different approach
# We'll create a simple lua-utils shim in the runtime

# Create app user
RUN groupadd -g 1001 appuser && \
    useradd -r -u 1001 -g appuser -m appuser

# Create necessary directories
RUN mkdir -p /app/.config/nvim /app/data /tmp/workdir /home/appuser

# Copy Neovim configuration
COPY .config/nvim/init.lua /app/.config/nvim/

# Copy docgen files, lua-utils shim, and static resources
COPY docgen/ /app/docgen/
COPY lua-utils.lua /usr/share/lua/5.1/
COPY res/ /app/res/

# Copy the built Go binary
COPY --from=go-builder /app/neorg-lambda /app/

# Set permissions and ownership
RUN chmod +x /app/neorg-lambda && chown -R appuser:appuser /app /tmp/workdir /opt/nvim /home/appuser

# Set working directory to /app so the Go binary can find docgen files
WORKDIR /app

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