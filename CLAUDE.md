# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is the Neorg Documentation Lambda, a document conversion service that transforms Neorg format files into Markdown documentation. The service is designed as a serverless function or containerized microservice that accepts tarball uploads containing Neorg files and returns ZIP archives of converted Markdown documentation.

## Development Commands

### Docker Operations
```bash
make install          # Clean + build Docker image
make run             # Complete pipeline: build + run + test conversion
make docker-build    # Build container with git hash tag  
make docker-run      # Run container with health checks
make docker-stop     # Stop running container
make docker-remove   # Remove container instance
make clean-test-data # Remove generated files (*.tar, *.zip, *.md)
```

### Container Registry
```bash
make push-to-registry # Push to GitHub Container Registry
make upload          # Build + push workflow
```

### Testing and Validation
```bash
make test-data       # Create test tarball and convert via API
make test-unzip      # Extract and examine converted output
make documentation   # Run Neovim conversion locally (headless mode)
```

### Docker Compose Deployment
```bash
docker-compose up -d    # Start service with environment config
docker-compose logs -f  # Monitor service logs  
docker-compose down     # Stop and remove containers
```

### Go Development
```bash
go mod tidy         # Update dependencies
go build ./serverless # Build binary
```

## Architecture

### High-Level Flow
1. **HTTP Handler** (`serverless/api.go:434-583`) - Accepts POST requests with tarball uploads via `x-auth-token` authentication
2. **Tarball Extraction** (`serverless/api.go:139-185`) - Secure extraction with path traversal protection, supports both .tar and .tar.gz formats
3. **Environment Setup** (`serverless/api.go:187-218`) - Copies Neovim configuration and Lua conversion scripts to project
4. **Documentation Generation** (`serverless/api.go:238-277`) - Executes `make documentation` using Neovim headless mode
5. **Archive Creation** (`serverless/api.go:305-432`) - Packages generated Markdown files into downloadable ZIP
6. **Response Streaming** (`serverless/api.go:564-582`) - Returns ZIP archive with proper HTTP headers

### Core Components

**Neovim Environment** (`.config/nvim/init.lua`):
- Lazy.nvim plugin manager with automatic installation
- TreeSitter with norg parser support  
- Neorg plugin with markdown export capabilities
- Kanagawa colorscheme for TreeSitter compatibility

**Conversion Engine** (`docgen/simple_norg_converter.lua`):
- Custom Lua script for Neorg to Markdown transformation
- Handles document metadata, headers, lists, code blocks, and inline formatting
- File I/O utilities for wiki directory structure creation

**Container Architecture** (Multi-stage Dockerfile):
- Stage 1: Go 1.25 builder for statically linked binary
- Stage 2: Ubuntu 22.04 runtime with Neovim 0.10+, C compiler for TreeSitter
- Security: Non-root user execution, resource limits, health checks

## Environment Configuration

### Required Variables
```bash
NEORG_DOCUMENTATION_AUTH_TOKEN=<token>  # API authentication (required)
PORT=2025                               # HTTP server port (default: 8080)
LOG_LEVEL=info                         # Logging verbosity (debug/info/warn/error)
LOG_FORMAT=text                        # Log output format (text/json)
```

### Docker Compose Variables
```bash
NEORG_AUTH_TOKEN=<token>               # Maps to NEORG_DOCUMENTATION_AUTH_TOKEN
DOMAIN=neorg-converter.domain.com      # Traefik/reverse proxy configuration
```

## Key File Locations

- **Main Service**: `serverless/api.go` - HTTP server and conversion orchestration
- **Conversion Scripts**: `docgen/` - Lua scripts for Neorg parsing and Markdown generation
- **Neovim Config**: `.config/nvim/init.lua` - Plugin setup for headless operation
- **Build Configuration**: `Makefile` - Docker operations and testing workflows
- **Container Setup**: `Dockerfile` - Multi-stage build with dependencies

## Security Considerations

- Authentication via `x-auth-token` header validation
- Path traversal protection in tarball extraction (`serverless/api.go:154`)
- Request timeout enforcement (5 minutes)
- Non-root container execution with resource limits
- Temporary directory cleanup after processing

## Archive Format Support

The service supports both compressed and uncompressed tar archives:
- **Uncompressed tar files** (.tar): Direct tar processing
- **Gzip-compressed tar files** (.tar.gz): Automatic gzip decompression before tar extraction
- Format detection is automatic based on file signatures

## API Usage

```bash
# Convert Neorg files to Markdown (supports both .tar and .tar.gz)
curl -X POST \
  -H "Content-Type: application/x-tar" \
  -H "x-auth-token: your-token" \
  --data-binary @project.tar.gz \
  http://localhost:2025 \
  --output converted_docs.zip

# Health check
curl http://localhost:2025/health
```

The service accepts both uncompressed (.tar) and gzip-compressed (.tar.gz) tarball input and returns ZIP archives containing generated Markdown files in a `wiki/` directory structure.