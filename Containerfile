FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /scraie ./cmd/scraie


FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /scraie /usr/local/bin/scraie
USER 65534:65534
CMD ["/usr/local/bin/scraie"]
