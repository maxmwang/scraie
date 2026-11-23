FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /scraie ./cmd/scraie
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /scraie-server ./cmd/server


FROM alpine:3.19 AS scraie
RUN apk --no-cache add ca-certificates
COPY --from=builder /scraie /usr/local/bin/scraie
USER 65534:65534
ENTRYPOINT ["/usr/local/bin/scraie"]


FROM alpine:3.19 AS server
RUN apk --no-cache add ca-certificates
COPY --from=builder /scraie-server /usr/local/bin/scraie-server
USER 65534:65534
ENTRYPOINT ["/usr/local/bin/scraie-server"]
