FROM arm64v8/alpine:3.23 AS build
RUN apk add --no-cache ca-certificates

FROM scratch

ENTRYPOINT ["/usr/bin/watchdog"]

# Copy CA certificates
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary from the build output
COPY watchdog /usr/bin/watchdog

