FROM alpine:3.21
RUN apk add --no-cache ca-certificates git
COPY drape /usr/local/bin/drape
ENTRYPOINT ["drape"]
