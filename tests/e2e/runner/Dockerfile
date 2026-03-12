# SeaPortal CLI E2E test runner
FROM alpine:3.21

RUN apk add --no-cache \
    bash \
    curl \
    jq \
    coreutils

# Copy pre-built seaportal binary (built by CI before docker build)
COPY seaportal /usr/local/bin/seaportal
RUN chmod +x /usr/local/bin/seaportal

RUN mkdir -p /results

WORKDIR /scenarios

CMD ["/scenarios/run-all.sh"]
