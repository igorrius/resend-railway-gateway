# Unosend Railway SMTP Gateway

A production-ready Go service that accepts SMTP messages and relays them to Unosend via HTTPS API. Built with clean architecture principles (DDD + SOLID) for maintainability and scalability.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/deploy/qbyFiP?referralCode=6mpzOK&utm_medium=integration&utm_source=template&utm_campaign=generic)

## Features
- **DDD + SOLID**: clear domain, application, and adapter layers
- **SMTP in, Unosend out**: uses `emersion/go-smtp` and Unosend REST API
- **Advanced MIME parsing**: supports multipart messages, HTML/text bodies, attachments
- **HTTP health endpoint**: `GET /health` on a second port for Railway healthchecks
- **Closed by default**: SMTP AUTH (PLAIN) required; optional sender-domain allowlist
- **Resilient delivery**: retries Unosend `429`/`5xx` with exponential backoff + jitter, honoring `Retry-After`
- **Correct SMTP semantics**: transient upstream failures → SMTP `4xx` (client retries), permanent → `5xx`
- **Graceful shutdown**: concurrent SMTP + HTTP listeners drained on `SIGTERM`
- **Structured logging**: JSON in production; logs message-id, from, recipient *domains*, Unosend id, latency, outcome — never the body or full recipient addresses
- **Config via ENV**: fail-fast validation at startup (no panics)
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
export UNOSEND_API_KEY=your_unosend_key
export SMTP_USER=user
export SMTP_PASS=pass
export SMTP_LISTEN_ADDR=:2525
export HEALTH_ADDR=:8080
go run ./cmd/gateway

# health check
curl -f http://localhost:8080/health   # {"status":"ok"}

# send an email via SMTP to localhost:2525 (AUTH required)
# swaks --server localhost:2525 --auth PLAIN --auth-user user --auth-password pass \
#   --from you@example.com --to them@example.com --header "Subject: Test" --body "Hello"
```

> To run without AUTH (private-network-only), set `SMTP_REQUIRE_AUTH=false`.

## Configuration

The service validates its configuration at startup and **fails fast** with a
clear message and non-zero exit (never a panic) if something required is missing.

### Required Environment Variables
- `UNOSEND_API_KEY` (required): API key from the [Unosend dashboard](https://www.unosend.co).
- `SMTP_USER` / `SMTP_PASS` (required when AUTH is enabled — the default): the
  credentials clients present via SMTP AUTH PLAIN. Startup fails if AUTH is
  required but these are unset.

### Optional Environment Variables
- `UNOSEND_API_URL` (default `https://api.unosend.co/v1/emails`): outbound API endpoint. Override this for a self-hosted or compatible Unosend deployment.
- `SMTP_LISTEN_ADDR` (default `:2525`): listen address for the SMTP server.
  - Example: `:2525`, `0.0.0.0:2525`, `localhost:2525`, or a bare port `2525`.
- `HEALTH_ADDR` (default `:8080`): listen address for the HTTP health server.
- `PORT`: if set (Railway), overrides **`HEALTH_ADDR`** as `":${PORT}"`.
  - Railway injects `PORT` for the public HTTP port it healthchecks. The SMTP
    listener is unaffected and stays on `SMTP_LISTEN_ADDR` (reached over the
    private network).
  - ⚠️ **Behavior change:** previously `PORT` overrode the SMTP port. It now
    drives the HTTP health port so `healthcheckPath=/health` works on Railway.
- `SEND_TIMEOUT_SECONDS` (default `15`): overall timeout for a single delivery,
  including all retries.
- `SEND_MAX_RETRIES` (default `3`): max retries (in addition to the first
  attempt) for transient Unosend failures (`429`, `5xx`, network/timeout).
- `SMTP_REQUIRE_AUTH` (default `true`): require SMTP AUTH before `MAIL FROM`.
  Set to `false` **only** when the listener is bound to a trusted private
  network (then `SMTP_USER`/`SMTP_PASS` are not required).
- `ALLOWED_FROM_DOMAINS` (default unset): comma-separated allowlist of envelope
  `MAIL FROM` domains (e.g. `example.com,mail.example.com`). When set, senders
  from other domains are rejected with SMTP `550`. Unset allows any domain.
- `LOG_LEVEL` (default `INFO`): logging verbosity.
  - Possible values: `DEBUG`, `INFO`, `WARN`, `ERROR`.

### Health endpoint & smoke test
`GET /health` returns `200 {"status":"ok"}` when the process is up and
`UNOSEND_API_KEY` is present, otherwise `503 {"status":"unavailable"}`.

```bash
# From another host/container:
curl -f http://<host>:8080/health

# Locally, using the built-in subcommand (also used as the Docker HEALTHCHECK):
./bin/unosend-railway-gateway healthcheck   # exit 0 = healthy, non-zero = not
```

## Project Structure
```
cmd/gateway               # main: concurrent SMTP + HTTP health, graceful shutdown
internal/domain           # core model, ports, typed send errors
internal/app              # orchestration service, structured logging
internal/adapters/smtp    # SMTP server: AUTH, allowlist, MIME parsing
internal/adapters/unosend  # Unosend client + retry/backoff transport
internal/adapters/health  # HTTP /health server
internal/config           # env config loader + startup validation
internal/logging          # slog setup
```

## Development

### Prerequisites
- Go 1.25 or later
- A Unosend API key ([get one here](https://www.unosend.co))

### Quick Start

1. **Clone the repository**
```bash
git clone https://github.com/igorrius/unosend-railway-gateway.git
cd unosend-railway-gateway
```

2. **Set up environment**
```bash
export UNOSEND_API_KEY=your_unosend_key
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
  --auth PLAIN --auth-user "$SMTP_USER" --auth-password "$SMTP_PASS" \
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
smtp.login('user', 'pass')  # SMTP_USER / SMTP_PASS
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

# Required variables
railway variables set UNOSEND_API_KEY=your_key
railway variables set SMTP_USER=your_smtp_user
railway variables set SMTP_PASS=your_strong_password

# Optional: restrict senders
railway variables set ALLOWED_FROM_DOMAINS=example.com

# Deploy
railway up
```

`railway.json` sets `healthcheckPath=/health`. Railway injects `PORT`, which the
gateway uses for its HTTP health server; Railway polls `/health` on that port to
gate deploys and restarts.

### Connecting your service over the private network

The SMTP listener is meant to be reached **only** over Railway's private
network, not exposed publicly. Point your calling service (Vaultwarden, an app's
mailer, etc.) at the gateway's internal hostname and SMTP port:

```
SMTP host: <gateway-service-name>.railway.internal
SMTP port: 2525            # SMTP_LISTEN_ADDR (not the public $PORT)
SMTP user: $SMTP_USER
SMTP pass: $SMTP_PASS
Encryption: none/STARTTLS-off (AUTH PLAIN is carried over the trusted private network)
```

Because the private network is trusted, AUTH PLAIN is transmitted without TLS
(`AllowInsecureAuth`). Do **not** publicly expose the SMTP port. If you must,
put it behind TLS/a tunnel and keep AUTH enabled. Only set
`SMTP_REQUIRE_AUTH=false` when you are certain the port is private-network-only.

### Docker Deployment

**Build the image:**
```bash
docker build -t unosend-railway-gateway .
```

**Run locally:**
```bash
docker run -p 2525:2525 \
  -e UNOSEND_API_KEY=your_key \
  -e SMTP_LISTEN_ADDR=:2525 \
  unosend-railway-gateway
```

**Production run:**
```bash
docker run -d \
  --name smtp-gateway \
  --restart unless-stopped \
  -p 2525:2525 \
  -e UNOSEND_API_KEY=your_key \
  unosend-railway-gateway
```

### Environment Variables for Railway

**Required:**
- `UNOSEND_API_KEY` - Your Unosend API key
- `SMTP_USER` / `SMTP_PASS` - SMTP AUTH credentials (required unless
  `SMTP_REQUIRE_AUTH=false`)

**Optional:**
- `UNOSEND_API_URL` - Unosend endpoint (default: `https://api.unosend.co/v1/emails`)
- `SMTP_LISTEN_ADDR` - SMTP listen address (default: `:2525`)
- `HEALTH_ADDR` - HTTP health address (default: `:8080`, overridden by `PORT`)
- `SEND_TIMEOUT_SECONDS` - Overall send timeout incl. retries (default: `15`)
- `SEND_MAX_RETRIES` - Transient-failure retries (default: `3`)
- `SMTP_REQUIRE_AUTH` - Require AUTH (default: `true`)
- `ALLOWED_FROM_DOMAINS` - Comma-separated sender-domain allowlist
- `LOG_LEVEL` - Control logging verbosity

**Port Configuration:**
- Railway automatically provides the `PORT` environment variable, which the
  gateway uses for its **HTTP health server** (`0.0.0.0:${PORT}`).
- The **SMTP** listener stays on `SMTP_LISTEN_ADDR` (default `:2525`) and is
  reached over the private network — see "Connecting your service over the
  private network" above.

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
│        Unosend Adapter                            │
│  - Converts to Unosend API                       │
│  - Sends via HTTPS                              │
└────────────────┬────────────────────────────────┘
                 │
┌────────────────▼────────────────────────────────┐
│              Unosend API                          │
└─────────────────────────────────────────────────┘
```

### Layer Responsibilities

- **Domain Layer** (`internal/domain`): Core business entities and interfaces
- **Application Layer** (`internal/app`): Orchestrates use cases
- **Adapter Layer** (`internal/adapters`): External integrations (SMTP, Unosend)
- **Config Layer** (`internal/config`): Environment-based configuration
- **Logging Layer** (`internal/logging`): Structured logging

## Troubleshooting

### Email not sending

1. **Check logs**: The gateway logs all operations at INFO level
   ```bash
   LOG_LEVEL=DEBUG make run
   ```

2. **Verify Unosend API key**: Ensure `UNOSEND_API_KEY` is set correctly
   ```bash
   echo $UNOSEND_API_KEY
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

- ✅ **Closed relay by default**: SMTP AUTH (PLAIN) is required; unauthenticated
  `MAIL FROM` is rejected with `502`. Startup fails if credentials are missing.
- ✅ **Sender allowlist**: set `ALLOWED_FROM_DOMAINS` to reject spoofed sender
  domains at `MAIL FROM` (`550`).
- ✅ **PII-aware logging**: logs record recipient *domains* only — never full
  recipient addresses or the message body.
- ✅ **Graceful shutdown** prevents message loss during restarts.
- ⚠️ Keep the SMTP port on the **private network**. AUTH PLAIN is sent without
  TLS there because the network is trusted; do not expose it publicly without
  TLS/a tunnel.
- ⚠️ `SMTP_REQUIRE_AUTH=false` disables AUTH — use only for private-network-only
  binds.
- ⚠️ Consider additional rate limiting for high-volume production use.

## Reliability

- Transient Unosend failures (`429`, `5xx`, network/timeout) are retried up to
  `SEND_MAX_RETRIES` times with exponential backoff + full jitter, honoring the
  `Retry-After` header.
- If retries are exhausted on a transient failure, the client receives an SMTP
  `4xx` (temporary) so it can retry later. Permanent failures (validation, auth)
  return SMTP `5xx`. The whole operation is bounded by `SEND_TIMEOUT_SECONDS`.

## Performance

- Handles multiple concurrent SMTP connections
- Efficient MIME parsing without full message buffering
- Configurable timeout for Unosend API calls
- Micro-benchmark: ~10k messages/second on modern hardware

## Contributing

Contributions are welcome! Please ensure:
- All tests pass: `make test`
- Code is formatted: `gofmt -s -w .`
- No linter errors: `make lint`

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details
