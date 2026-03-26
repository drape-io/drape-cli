FROM alpine:3.21
RUN apk add --no-cache ca-certificates git
RUN adduser -D -u 10001 drape
COPY drape /usr/local/bin/drape
USER drape
ENTRYPOINT ["drape"]
