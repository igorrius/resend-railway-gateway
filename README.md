# Resend Railway SMTP Gateway

A production-ready Go service that accepts SMTP messages and relays them to Resend via HTTPS API. Built with clean architecture principles (DDD + SOLID) for maintainability and scalability.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/deploy/qbyFiP?referralCode=6mpzOK&utm_medium=integration&utm_source=template&utm_campaign=generic)

## Features
- **DDD + SOLID**: clear domain, application, and adapter layers
- **SMTP in, Resend out**: uses `emersion/go-smtp` and Resend REST API
- **Advanced MIME parsing**: supports multipart messages, HTML/text bodies, attachments
- **Graceful shutdown**: proper signal handling for clean container restarts
- **Structured logging**: JSON in production, text in development
- **Config via ENV**: `RESEND_API_KEY`, `SMTP_LISTEN_ADDR`, `SEND_TIMEOUT_SECONDS`
- **Tests and Benchmarks**: unit tests and micro-benchmark for the send path
- **Docker & Railway**: ready-to-deploy container and `railway.json`

## Supported Email Features
- ✅ Plain text emails
- ✅ HTML emails
- ✅ Multipart emails (text + HTML)
- ✅ Attachments (inline and regular)
- ✅ CC, BCC, and Reply-To headers
- ✅ Base64 and quoted-printable content transfer encoding
- ✅ Custom headers

## Quick start
```bash
export RESEND_API_KEY=your_resend_key
export SMTP_LISTEN_ADDR=:2525
go run ./cmd/gateway
# send an email via SMTP to localhost:2525
# example (swaks):
# swaks --server localhost:2525 --from you@example.com --to them@example.com --data "Subject: Test\n\nHello"
```

## Configuration

### Required Environment Variables
- `RESEND_API_KEY` (required): API key for Resend

### Optional Environment Variables
- `SMTP_LISTEN_ADDR` (default `:2525`): listen address for SMTP server
  - Example: `:2525`, `0.0.0.0:2525`, `localhost:2525`
- `SEND_TIMEOUT_SECONDS` (default `15`): timeout for send pipeline in seconds
  - Maximum time to wait for Resend API response before failing
- `PORT`: if set (Railway), overrides SMTP port as `":${PORT}"`
  - Automatically used by Railway for dynamic port allocation
- `LOG_LEVEL` (default `INFO`): logging verbosity
  - Possible values: `DEBUG`, `INFO`, `WARN`, `ERROR`

## Project Structure
```
cmd/gateway          # main
internal/domain      # core model and ports
internal/app         # orchestration service
internal/adapters    # smtp server, resend client
internal/config      # env config loader
```

## Development

### Prerequisites
- Go 1.25 or later
- A Resend API key ([get one here](https://resend.com))

### Quick Start

1. **Clone the repository**
```bash
git clone https://github.com/igorrius/resend-railway-gateway.git
cd resend-railway-gateway
```

2. **Set up environment**
```bash
export RESEND_API_KEY=your_resend_key
export SMTP_LISTEN_ADDR=:2525
```

3. **Run the gateway**
```bash
make run
# or
go run ./cmd/gateway
```

### Testing

```bash
# Run all tests
make test

# Run benchmarks
make bench

# Run with coverage
go test -race -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out
```

### Available Make Commands
- `make build` - Build the binary
- `make test` - Run tests with race detection
- `make bench` - Run benchmarks
- `make lint` - Run golangci-lint
- `make run` - Run the gateway locally
- `make clean` - Remove build artifacts

### Testing with Real SMTP Client

You can test the gateway using a variety of SMTP clients:

**Using swaks:**
```bash
swaks --server localhost:2525 \
  --from sender@example.com \
  --to recipient@example.com \
  --header "Subject: Test Email" \
  --body "Hello from the SMTP gateway!"
```

**Using curl (for raw SMTP):**
```bash
curl smtp://localhost:2525
# Then manually type SMTP commands
```

**Using Python:**
```python
import smtplib
from email.message import EmailMessage

msg = EmailMessage()
msg['From'] = 'sender@example.com'
msg['To'] = 'recipient@example.com'
msg['Subject'] = 'Test Email'
msg.set_content('Hello from Python!')

smtp = smtplib.SMTP('localhost', 2525)
smtp.send_message(msg)
smtp.quit()
```

## Deployment

### Railway Deployment

1. **One-click deploy**: Click the Railway button above
2. **Manual deploy**:
```bash
# Install Railway CLI
npm i -g @railway/cli

# Login and initialize
railway login
railway init

# Add environment variable
railway variables set RESEND_API_KEY=your_key

# Deploy
railway up
```

### Docker Deployment

**Build the image:**
```bash
docker build -t resend-railway-gateway .
```

**Run locally:**
```bash
docker run -p 2525:2525 \
  -e RESEND_API_KEY=your_key \
  -e SMTP_LISTEN_ADDR=:2525 \
  resend-railway-gateway
```

**Production run:**
```bash
docker run -d \
  --name smtp-gateway \
  --restart unless-stopped \
  -p 2525:2525 \
  -e RESEND_API_KEY=your_key \
  resend-railway-gateway
```

### Environment Variables for Railway

**Required:**
- `RESEND_API_KEY` - Your Resend API key

**Optional:**
- `SMTP_LISTEN_ADDR` - Override listen address (default: `:2525`)
- `SEND_TIMEOUT_SECONDS` - Override timeout (default: `15`)
- `LOG_LEVEL` - Control logging verbosity

**Port Configuration:**
- Railway automatically provides `PORT` environment variable
- The gateway listens on `0.0.0.0:${PORT}` when deployed to Railway

## Architecture

The service follows a clean architecture with clear separation of concerns:

```
┌─────────────────────────────────────────────────┐
│                  SMTP Clients                   │
└────────────────┬────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────┐
│        SMTP Adapter (go-smtp)                   │
│  - Accepts SMTP connections                     │
│  - Parses MIME messages                        │
│  - Converts to domain model                     │
└────────────────┬────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────┐
│         Application Service                      │
│  - Orchestrates business logic                  │
│  - Handles timeouts                             │
│  - Logs operations                              │
└────────────────┬────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────┐
│     Domain Model (Email, Ports)                 │
│  - Validates data                               │
│  - Business rules                                │
└────────────────┬────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────┐
│        Resend Adapter                            │
│  - Converts to Resend API                       │
│  - Sends via HTTPS                              │
└────────────────┬────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────┐
│              Resend API                          │
└─────────────────────────────────────────────────┘
```

### Layer Responsibilities

- **Domain Layer** (`internal/domain`): Core business entities and interfaces
- **Application Layer** (`internal/app`): Orchestrates use cases
- **Adapter Layer** (`internal/adapters`): External integrations (SMTP, Resend)
- **Config Layer** (`internal/config`): Environment-based configuration
- **Logging Layer** (`internal/logging`): Structured logging

## Troubleshooting

### Email not sending

1. **Check logs**: The gateway logs all operations at INFO level
   ```bash
   LOG_LEVEL=DEBUG make run
   ```

2. **Verify Resend API key**: Ensure `RESEND_API_KEY` is set correctly
   ```bash
   echo $RESEND_API_KEY
   ```

3. **Test SMTP connection**:
   ```bash
   telnet localhost 2525
   # or
   nc localhost 2525
   ```

4. **Check Railway logs**: View deployment logs in Railway dashboard

### Timeout issues

If emails are timing out, increase the timeout:
```bash
export SEND_TIMEOUT_SECONDS=30
```

### Port binding issues

If running in a container, ensure the port is exposed:
```bash
docker run -p 2525:2525 ...
```

## Security Considerations

- ⚠️ The SMTP server does not require authentication by default
- ⚠️ Consider running behind a firewall or VPN
- ⚠️ Implement rate limiting for production use
- ✅ Graceful shutdown prevents message loss
- ✅ Structured logging for security auditing

## Performance

- Handles multiple concurrent SMTP connections
- Efficient MIME parsing without full message buffering
- Configurable timeout for Resend API calls
- Micro-benchmark: ~10k messages/second on modern hardware

## Contributing

Contributions are welcome! Please ensure:
- All tests pass: `make test`
- Code is formatted: `gofmt -s -w .`
- No linter errors: `make lint`

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details
