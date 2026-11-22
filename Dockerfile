FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /pocketkv ./cmd/pocketkv

FROM alpine:3.22
RUN adduser -D -H -u 10001 pocketkv && mkdir /data && chown pocketkv:pocketkv /data
COPY --from=build /pocketkv /usr/local/bin/pocketkv
# Fixed inside the container: listen on 8080, keep data on the /data volume.
ENV ADDR=:8080 DATA_FILE=/data/pocketkv.json
USER pocketkv
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["pocketkv"]
