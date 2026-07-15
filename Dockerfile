FROM golang:1.25-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/app ./cmd/gateway

FROM gcr.io/distroless/base-debian12
COPY --from=builder /out/app /app
# SMTP listener (private network) and HTTP health/port (Railway $PORT).
EXPOSE 2525
EXPOSE 8080
USER nonroot:nonroot
# distroless has no shell/curl; use the built-in healthcheck subcommand.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/app", "healthcheck"]
ENTRYPOINT ["/app"]
