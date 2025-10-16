# Neorg Documentation Lambda

A containerized microservice that converts Neorg (`.norg`) documents to Markdown using Neovim with the Neorg plugin. Designed for homelab deployment and integration with GitHub Actions for automated documentation publishing.

## Overview

This service provides a REST API that:
- Accepts ZIP archives containing `.norg` files
- Converts them to Markdown using Neovim's headless mode with the Neorg plugin
- Returns a ZIP archive of the converted Markdown files
- Supports concurrent processing with worker pools for performance
- Includes proper authentication and error handling

## Prerequisites

- Docker and Docker Compose (for deployment)
- Go 1.21+ (for development)
- Access to your homelab environment

## Quick Start

### 1. Deploy to Homelab

```bash
# Clone the repository
git clone <repository-url>
cd Neorg.Documentation.Lambda

# Build and run with Docker
docker build -t neorg-lambda .
docker run -d \
  --name neorg-converter \
  -p 8080:8080 \
  -e NEORG_DOCUMENTATION_AUTH_TOKEN=your-secure-token-here \
  --restart unless-stopped \
  neorg-lambda
```

### 2. Docker Compose (Recommended)

Create a `docker-compose.yml`:

```yaml
version: '3.8'

services:
  neorg-converter:
    build: .
    container_name: neorg-converter
    ports:
      - "8080:8080"
    environment:
      - NEORG_DOCUMENTATION_AUTH_TOKEN=your-secure-token-here
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

Then deploy:

```bash
docker-compose up -d
```

## API Documentation

### Authentication

All requests require the `x-auth-token` header with your configured token:

```bash
curl -H "x-auth-token: your-secure-token-here" http://your-homelab:8080/health
```

### Endpoints

#### `POST /`

Converts Neorg files to Markdown.

**Request:**
- Method: `POST`
- Headers: 
  - `x-auth-token: your-token`
  - `Content-Type: application/zip` (or `application/octet-stream`)
- Body: ZIP archive containing `.norg` files

**Response:**
- Success (200): ZIP archive containing converted `.md` files
- Error (400/401/500): JSON error response

**Example:**

```bash
# Create a zip of your .norg files
zip -r docs.zip *.norg

# Convert them
curl -X POST \
  -H "x-auth-token: your-secure-token-here" \
  -H "Content-Type: application/zip" \
  --data-binary @docs.zip \
  -o converted.zip \
  http://your-homelab:8080/

# Extract converted files
unzip converted.zip
```

#### `GET /health`

Health check endpoint.

**Response:** `200 OK` with body `"OK"`

## GitHub Actions Integration

### Basic Workflow

Create `.github/workflows/publish-docs.yml`:

```yaml
name: Publish Documentation

on:
  push:
    branches: [main]
    paths: ['docs/**/*.norg']
  workflow_dispatch:

jobs:
  convert-and-publish:
    runs-on: ubuntu-latest
    
    steps:
    - name: Checkout repository
      uses: actions/checkout@v4
      
    - name: Create docs archive
      run: |
        cd docs
        zip -r ../docs.zip *.norg **/*.norg
        
    - name: Convert Neorg to Markdown
      run: |
        curl -X POST \
          -H "x-auth-token: ${{ secrets.NEORG_CONVERTER_TOKEN }}" \
          -H "Content-Type: application/zip" \
          --data-binary @docs.zip \
          -o converted.zip \
          ${{ secrets.NEORG_CONVERTER_URL }}
          
    - name: Extract converted files
      run: |
        mkdir -p wiki
        cd wiki
        unzip ../converted.zip
        
    - name: Publish to Wiki
      uses: SwiftDocOrg/github-wiki-publish-action@v1
      with:
        path: "wiki"
      env:
        GH_PERSONAL_ACCESS_TOKEN: ${{ secrets.GH_PAT }}
```

### Advanced Workflow with Error Handling

```yaml
name: Advanced Documentation Publishing

on:
  push:
    branches: [main]
    paths: ['docs/**/*.norg']

jobs:
  convert-docs:
    runs-on: ubuntu-latest
    
    steps:
    - name: Checkout repository
      uses: actions/checkout@v4
      
    - name: Find Neorg files
      id: find-files
      run: |
        if find docs -name "*.norg" -type f | head -1 | grep -q .; then
          echo "found=true" >> $GITHUB_OUTPUT
          find docs -name "*.norg" -type f | wc -l > norg_count.txt
          echo "count=$(cat norg_count.txt)" >> $GITHUB_OUTPUT
        else
          echo "found=false" >> $GITHUB_OUTPUT
          echo "No .norg files found to convert"
        fi
        
    - name: Create docs archive
      if: steps.find-files.outputs.found == 'true'
      run: |
        cd docs
        find . -name "*.norg" -type f -print0 | zip -0 ../docs.zip -@
        ls -la ../docs.zip
        
    - name: Convert Neorg to Markdown
      if: steps.find-files.outputs.found == 'true'
      id: convert
      run: |
        response=$(curl -w "%{http_code}" -X POST \
          -H "x-auth-token: ${{ secrets.NEORG_CONVERTER_TOKEN }}" \
          -H "Content-Type: application/zip" \
          --data-binary @docs.zip \
          -o converted.zip \
          ${{ secrets.NEORG_CONVERTER_URL }})
          
        if [ "$response" != "200" ]; then
          echo "Conversion failed with HTTP $response"
          exit 1
        fi
        
        echo "Conversion successful, checking output..."
        ls -la converted.zip
        
    - name: Extract and organize converted files
      if: steps.convert.outcome == 'success'
      run: |
        mkdir -p wiki
        cd wiki
        unzip ../converted.zip
        
        # List converted files
        echo "Converted files:"
        ls -la
        
        # Optional: Add index file
        echo "# Documentation" > README.md
        echo "" >> README.md
        echo "Generated from Neorg files on $(date)" >> README.md
        echo "" >> README.md
        for file in *.md; do
          if [ "$file" != "README.md" ]; then
            echo "- [$file](./$file)" >> README.md
          fi
        done
        
    - name: Publish to Wiki
      if: steps.convert.outcome == 'success'
      uses: SwiftDocOrg/github-wiki-publish-action@v1
      with:
        path: "wiki"
      env:
        GH_PERSONAL_ACCESS_TOKEN: ${{ secrets.GH_PAT }}
        
    - name: Upload artifacts on failure
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: debug-files
        path: |
          docs.zip
          converted.zip
        retention-days: 7
```

### Required Secrets

In your GitHub repository settings, add these secrets:

- `NEORG_CONVERTER_TOKEN`: Your authentication token
- `NEORG_CONVERTER_URL`: Your homelab service URL (e.g., `http://homelab.local:8080`)
- `GH_PAT`: GitHub Personal Access Token with wiki permissions

## Configuration

### Environment Variables

- `NEORG_DOCUMENTATION_AUTH_TOKEN`: **Required**. Authentication token for API access
- `XDG_CONFIG_HOME`: Path to Neovim config (default: `/app/.config`)
- `XDG_DATA_HOME`: Path to Neovim data (default: `/app/data`)

### Neovim Configuration

The service uses the bundled Neovim configuration in `.config/nvim/init.lua`, which includes:

- Lazy.nvim package manager
- Neorg plugin with export functionality
- Kanagawa colorscheme (required for Treesitter)
- Treesitter for syntax highlighting

To customize the Neovim configuration, modify `.config/nvim/init.lua` and rebuild the container.

## Development

### Local Development

```bash
# Install dependencies
go mod tidy

# Run locally (requires Neovim with Neorg plugin)
export NEORG_DOCUMENTATION_AUTH_TOKEN=test-token
go run serverless/api.go
```

### Testing

```bash
# Test health endpoint
curl http://localhost:8080/health

# Test conversion (create test.norg first)
echo "* Test Neorg File" > test.norg
zip test.zip test.norg
curl -X POST \
  -H "x-auth-token: test-token" \
  --data-binary @test.zip \
  -o result.zip \
  http://localhost:8080/
```

## Homelab Deployment Tips

### Reverse Proxy Setup (Nginx)

```nginx
server {
    listen 80;
    server_name neorg-converter.local;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # Increase timeouts for large conversions
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
        
        # Increase body size for large zip files
        client_max_body_size 50M;
    }
}
```

### Monitoring

Add monitoring to your deployment:

```yaml
# In docker-compose.yml
services:
  neorg-converter:
    # ... existing config
    labels:
      - "com.docker.compose.project=homelab"
      - "service.name=neorg-converter"
      
  # Optional: Add monitoring
  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
```

## Troubleshooting

### Common Issues

1. **"Unauthorized" errors**
   - Check that `NEORG_DOCUMENTATION_AUTH_TOKEN` is set
   - Verify the token matches in both client and server

2. **"No .norg files found"**
   - Ensure your ZIP contains files with `.norg` extension
   - Check that files aren't nested too deeply

3. **Conversion timeouts**
   - Large files may take time; the service has a 5-minute timeout
   - Consider splitting large documents

4. **Container fails to start**
   - Check logs: `docker logs neorg-converter`
   - Ensure port 8080 isn't already in use

### Logs

```bash
# View container logs
docker logs -f neorg-converter

# Check health
docker exec neorg-converter wget -q --spider http://localhost:8080/health
```

### Performance Tuning

For better performance with large batches:

- Increase worker count in the code (currently set to max 3)
- Allocate more CPU/memory to the container
- Use SSD storage for faster temporary file operations

## Security Considerations

- Use a strong, unique authentication token
- Consider running behind a reverse proxy with TLS
- Limit access to your homelab network
- Regularly update the container image
- Monitor logs for suspicious activity

## License

This project is licensed under the GNU General Public License v3.0 (GPL-3.0).

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for more details.

You should have received a copy of the GNU General Public License along with this program. If not, see <https://www.gnu.org/licenses/>.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test locally
5. Submit a pull request

## Support

For issues and questions:
- Check the troubleshooting section above
- Review container logs
- Open an issue in the repository