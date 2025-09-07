FROM alpine:3.20
RUN adduser -D -g '' app && apk add --no-cache ca-certificates tzdata
USER app
WORKDIR /home/app

COPY bin/kadlab /usr/local/bin/kadlab
EXPOSE 4000
CMD ["kadlab"]
