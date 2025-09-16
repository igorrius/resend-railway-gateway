FROM golang:1.24-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/app ./cmd/gateway

FROM gcr.io/distroless/base-debian12
COPY --from=builder /out/app /app
EXPOSE 2525
USER nonroot:nonroot
ENTRYPOINT ["/app"]
