FROM gcr.io/distroless/static-debian12:nonroot
COPY drape /usr/local/bin/drape
USER nonroot
ENTRYPOINT ["drape"]
