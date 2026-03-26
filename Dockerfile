FROM cgr.dev/chainguard/git:latest-root AS base
FROM base
COPY drape /usr/local/bin/drape
USER git
ENTRYPOINT ["drape"]
