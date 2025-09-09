# Resend Railway SMTP Gateway

Small Go service that accepts SMTP messages and relays them to Resend via HTTPS API.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/deploy/qbyFiP?referralCode=6mpzOK&utm_medium=integration&utm_source=template&utm_campaign=generic)

## Features
- **DDD + SOLID**: clear domain, application, and adapter layers
- **SMTP in, Resend out**: uses `emersion/go-smtp` and Resend REST API
- **Config via ENV**: `RESEND_API_KEY`, `SMTP_LISTEN_ADDR`, `SEND_TIMEOUT_SECONDS`
- **Tests and Benchmarks**: unit tests and micro-benchmark for the send path
- **Docker & Railway**: ready-to-deploy container and `railway.json`

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
- `RESEND_API_KEY` (required): API key for Resend
- `SMTP_LISTEN_ADDR` (default `:2525`): listen address for SMTP server
- `SEND_TIMEOUT_SECONDS` (default `15`): timeout for send pipeline
- `PORT`: if set (Railway), overrides SMTP port as `":${PORT}"`

## Project Structure
```
cmd/gateway          # main
internal/domain      # core model and ports
internal/app         # orchestration service
internal/adapters    # smtp server, resend client
internal/config      # env config loader
```

## Development
```bash
go mod tidy
make run
make test
make bench
```

## Deployment
Build a container and deploy to Railway.
```bash
docker build -t resend-railway-gateway .
```

### Required variables on Railway
- `RESEND_API_KEY`

### Exposed port
- TCP: `${PORT}` (Railway will inject `PORT`)

## License
MIT
