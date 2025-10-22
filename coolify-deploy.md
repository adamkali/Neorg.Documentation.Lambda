# Coolify Deployment Guide

This guide explains how to deploy the Neorg Documentation Lambda service using Coolify.

## Prerequisites

- Coolify instance running and accessible
- Domain name configured (optional but recommended)
- Git repository access from Coolify

## Deployment Steps

### 1. Create New Resource in Coolify

1. Go to your Coolify dashboard
2. Click "New Resource"
3. Select "Docker Compose" 
4. Choose "Public Repository" or connect your Git provider

### 2. Repository Configuration

- **Repository URL**: `https://github.com/yourusername/neorg-documentation-lambda`
- **Branch**: `main` (or your preferred branch)
- **Build Pack**: Docker Compose
- **Docker Compose Location**: `./docker-compose.yml`

### 3. Environment Variables

Set these environment variables in Coolify:

| Variable | Value | Description |
|----------|-------|-------------|
| `NEORG_AUTH_TOKEN` | `your-secure-token` | **Required** - API authentication token |
| `DOMAIN` | `neorg.yourdomain.com` | Your domain for the service |
| `LOG_LEVEL` | `info` | Logging level (debug/info/warn/error) |
| `LOG_FORMAT` | `text` | Log format (text/json) |

### 4. Domain Configuration

1. In Coolify, go to your application settings
2. Add your domain: `neorg.yourdomain.com`
3. Enable SSL/TLS (Let's Encrypt)
4. Coolify will automatically configure Traefik reverse proxy

### 5. Deploy

1. Click "Deploy" in Coolify
2. Monitor the build logs
3. Once deployed, the service will be available at your configured domain

## Health Check

The service includes a health check endpoint. Coolify will automatically monitor:
- **Endpoint**: `http://localhost:2025/health`
- **Interval**: 30 seconds
- **Timeout**: 10 seconds
- **Retries**: 3

## Usage After Deployment

### Test the Service

```bash
# Create test files
echo "* Test Document" > test.norg
tar -cvf test.tar test.norg

# Test the deployed service
curl -X POST \
  -H "Content-Type: application/x-tar" \
  -H "x-auth-token: your-secure-token" \
  --data-binary @test.tar \
  https://neorg.yourdomain.com \
  --output converted.zip

# Extract results
unzip converted.zip
```

### GitHub Actions Integration

Update your GitHub secrets with your Coolify deployment:

- `NEORG_CONVERTER_TOKEN`: Your `NEORG_AUTH_TOKEN` value
- `NEORG_CONVERTER_URL`: `https://neorg.yourdomain.com`

## Resource Limits

The docker-compose.yml includes resource limits suitable for most use cases:

- **Memory**: 512MB limit, 256MB reserved
- **CPU**: 0.5 cores limit, 0.25 cores reserved

Adjust these in the docker-compose.yml if needed for your workload.

## Monitoring and Logs

### View Logs in Coolify
1. Go to your application in Coolify
2. Click on the "Logs" tab
3. View real-time application logs

### Log Configuration
- Default log level: `info`
- Set `LOG_LEVEL=debug` for detailed debugging
- Set `LOG_FORMAT=json` for structured logging

## Scaling

### Horizontal Scaling
To handle more traffic, you can scale the service:

1. In Coolify, go to your application
2. Adjust the replica count
3. Coolify will load balance between instances

### Vertical Scaling
Modify resource limits in `docker-compose.yml`:

```yaml
deploy:
  resources:
    limits:
      memory: 1G        # Increase memory
      cpus: '1.0'       # Increase CPU
```

## Security Considerations

1. **Strong Auth Token**: Use a cryptographically strong token
2. **HTTPS Only**: Always use HTTPS in production
3. **Network Isolation**: Coolify networks provide isolation
4. **Regular Updates**: Keep the container image updated

## Troubleshooting

### Common Issues

**Service won't start:**
- Check environment variables are set correctly
- Verify the `NEORG_AUTH_TOKEN` is configured
- Review build logs in Coolify

**Health check failing:**
- Ensure port 2025 is exposed correctly
- Check if the service is listening on all interfaces (0.0.0.0)
- Verify no firewall blocking internal health checks

**Authentication errors:**
- Verify the `x-auth-token` header matches `NEORG_AUTH_TOKEN`
- Check for extra whitespace in token values

### Debug Mode

Enable debug logging by setting `LOG_LEVEL=debug` in Coolify environment variables.

## Updates and Maintenance

### Automatic Updates
Configure Coolify to automatically redeploy on git pushes:

1. Go to application settings in Coolify
2. Enable "Automatic Deployment"
3. Configure webhook if using external Git provider

### Manual Updates
1. Go to your application in Coolify
2. Click "Redeploy" to pull latest changes
3. Monitor deployment logs

## Support

- Check application logs in Coolify for errors
- Review the main README.md for API documentation
- Monitor resource usage in Coolify dashboard