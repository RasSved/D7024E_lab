# --- build ---
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/node ./main.go

# --- run ---
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/node /app/node
ENV PORT=9999 AUTO_ACK=1
ENTRYPOINT ["/app/node"]
