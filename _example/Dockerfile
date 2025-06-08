FROM golang:1.24.3 AS builder
WORKDIR /workspace
ENV CGO_ENABLED=0
ENV GOOS=linux
COPY . .
RUN go build -a -installsuffix cgo -o runtime_binary ./cmd

# Production stage
FROM scratch

# Build args
ARG COMMIT_SHA
ENV COMMIT_SHA="${COMMIT_SHA:-No COMMIT_SHA}"

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /workspace/runtime_binary ./
EXPOSE 8080
ENTRYPOINT ["./runtime_binary"]
