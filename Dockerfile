# ---- build stage ----
FROM golang:1.23-alpine AS build
RUN apk add --no-cache git
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /bin/kademlia ./cmd

# ---- runtime stage ----
FROM alpine:3.20
WORKDIR /data

# optional: add bash/netcat for debugging
RUN apk add --no-cache bash busybox-extras

COPY --from=build /bin/kademlia /bin/kademlia

ENTRYPOINT ["/bin/kademlia"]
