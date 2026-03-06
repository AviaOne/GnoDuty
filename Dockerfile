# GnoDuty Dockerfile
# Adapted from Tenderduty (https://github.com/blockpane/tenderduty)

# 1st stage, build app
FROM golang:1.21 as builder
RUN apt-get update && apt-get -y upgrade
COPY . /build/app
WORKDIR /build/app
RUN go get ./... && go build -ldflags "-s -w" -trimpath -o gnoduty main.go

# 2nd stage, create a user and install SSL libraries needed for TLS connections
FROM debian:12 AS ssl
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get -y upgrade && apt-get install -y ca-certificates && \
    addgroup --gid 26657 --system gnoduty && adduser -uid 26657 --ingroup gnoduty --system --home /var/lib/gnoduty gnoduty

# 3rd and final stage, copy the minimum parts into a scratch container
FROM scratch
COPY --from=ssl /etc/ca-certificates /etc/ca-certificates
COPY --from=ssl /etc/ssl /etc/ssl
COPY --from=ssl /usr/share/ca-certificates /usr/share/ca-certificates
COPY --from=ssl /usr/lib /usr/lib
COPY --from=ssl /lib /lib
COPY --from=ssl /lib64 /lib64
COPY --from=ssl /etc/passwd /etc/passwd
COPY --from=ssl /etc/group /etc/group
COPY --from=ssl --chown=gnoduty:gnoduty /var/lib/gnoduty /var/lib/gnoduty
COPY --from=builder /build/app/gnoduty /bin/gnoduty
COPY --from=builder /build/app/example-config.yml /var/lib/gnoduty
USER gnoduty
WORKDIR /var/lib/gnoduty
ENTRYPOINT ["/bin/gnoduty"]
