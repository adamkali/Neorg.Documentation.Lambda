# Neorg Documentation Lambda

A containerized document conversion service that transforms [Neorg](https://github.com/nvim-neorg/neorg) format files into Markdown documentation. Built as a serverless function using Go and Neovim.

## Features

- 🔄 **Format Support**: Converts `.norg` files to `.md` with full syntax preservation
- 📦 **Archive Processing**: Supports both `.tar` and `.tar.gz` input archives
- 🐳 **Containerized**: Docker-first deployment with health checks
- 🔐 **Secure**: Token-based authentication and path traversal protection
- ⚡ **Fast**: Neovim headless mode for efficient conversion
- 📝 **Comprehensive Logging**: Structured logging with request tracing

## Quick Start

### Using Docker Compose

1. **Clone and configure**:
   ```bash
   git clone <repository-url>
   cd Neorg.Documentation.Lambda
   cp .env.example .env
   # Edit .env with your authentication token
   ```

2. **Start the service**:
   ```bash
   docker-compose up -d
   ```

3. **Convert documents**:
   ```bash
   # Create a tarball of your Neorg files
   tar -czf my-docs.tar.gz docs/
   
   # Convert to Markdown
   curl -X POST \
     -H "Content-Type: application/x-tar" \
     -H "x-auth-token: your-token-here" \
     --data-binary @my-docs.tar.gz \
     http://localhost:2025 \
     --output converted-docs.zip
   
   # Extract converted files
   unzip converted-docs.zip
   ```

### Using Make Commands

```bash
# Build and run complete pipeline
make run

# Build Docker image
make docker-build

# Start container
make docker-run

# Test with sample data
make test-data && make test-unzip

# Clean up
make clean-test-data
```

## API Reference

### Convert Documents

**Endpoint**: `POST /`

**Headers**:
- `Content-Type: application/x-tar`
- `x-auth-token: <your-token>`

**Request Body**: Raw binary data (tar or tar.gz archive)

**Response**: ZIP archive containing converted Markdown files

**Example**:
```bash
curl -X POST \
  -H "Content-Type: application/x-tar" \
  -H "x-auth-token: secret-token" \
  --data-binary @project.tar.gz \
  http://localhost:2025 \
  --output docs.zip
```

### Health Check

**Endpoint**: `GET /health`

**Response**: `200 OK` if service is healthy

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `NEORG_DOCUMENTATION_AUTH_TOKEN` | API authentication token | - | ✅ |
| `PORT` | HTTP server port | `8080` | ❌ |
| `LOG_LEVEL` | Logging level (debug/info/warn/error) | `info` | ❌ |
| `LOG_FORMAT` | Log format (text/json) | `text` | ❌ |

## Supported Neorg Features

The converter handles the following Neorg syntax:

- **Headers**: `*`, `**`, `***` → `#`, `##`, `###`
- **Lists**: `-`, `--`, `---` and `~`, `~~`, `~~~`
- **Text Formatting**:
  - Bold: `{* text *}` → `**text**`
  - Italic: `{/ text /}` → `*text*`
  - Code: `{` text `}` → `` `text` ``
  - Strikethrough: `{- text -}` → `~~text~~`
- **Links**: `{url}[text]` → `[text](url)`
- **Code Blocks**: `@code lang` ... `@end`
- **Document Metadata**: `@document.meta` with title extraction

## Development

### Prerequisites

- Docker and Docker Compose
- Go 1.25+ (for local development)
- Make

### Project Structure

```
.
├── serverless/         # Go HTTP server and API handlers
├── docgen/            # Lua conversion scripts
├── .config/nvim/      # Neovim configuration for headless mode
├── res/               # Static resources
├── Dockerfile         # Multi-stage container build
├── docker-compose.yml # Service orchestration
├── Makefile          # Build automation
└── README.md         # This file
```

### Local Development

```bash
# Install dependencies
go mod tidy

# Build binary
go build ./serverless

# Run tests (with container)
make run

# View logs
make docker-logs
```

### Architecture

1. **HTTP Handler**: Receives tarball uploads with authentication
2. **Archive Extraction**: Secure extraction supporting tar/tar.gz formats
3. **Neovim Processing**: Headless Neovim with Neorg plugins converts files
4. **Response Packaging**: Generated Markdown files are ZIP-archived and returned

## Deployment

### Docker Registry

Push to GitHub Container Registry:

```bash
make push-to-registry
```

### Production Configuration

Set environment variables in your deployment platform:

```bash
NEORG_DOCUMENTATION_AUTH_TOKEN=your-secure-token
LOG_LEVEL=info
LOG_FORMAT=json
PORT=8080
```

## Security

- **Authentication**: All requests require valid `x-auth-token` header
- **Path Traversal Protection**: Archive extraction validates file paths
- **Resource Limits**: Container memory and CPU limits prevent abuse
- **Request Timeouts**: 5-minute timeout for conversion operations
- **Non-root Execution**: Container runs as unprivileged user

## Troubleshooting

### Common Issues

**"Invalid tar header" error**:
- Ensure you're sending a valid tar or tar.gz file
- Check Content-Type header is `application/x-tar`

**"Unauthorized" response**:
- Verify `x-auth-token` header matches `NEORG_DOCUMENTATION_AUTH_TOKEN`

**"Service unavailable" health check**:
- Container may still be starting (Neovim plugin installation)
- Check logs: `docker logs <container-name>`

### Debug Logging

Enable debug logging to see detailed conversion process:

```bash
docker run -e LOG_LEVEL=debug neorg.documentation.lambda
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## Related Projects

- [Neorg](https://github.com/nvim-neorg/neorg) - The note-taking framework this service converts from
- [Neovim](https://neovim.io/) - The editor engine powering the conversion