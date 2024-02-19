FROM alpine:3.18.3

RUN apk --update --no-cache add ca-certificates bash vim curl python3 tzdata rsync

# users:x:100:games already created by system
RUN mkdir -p /data/home && \
    adduser -D -S -G users -u 30000 -g "" -s /bin/bash -h /data/home/user00 user00

WORKDIR /usr/local/bscp

COPY docker-entrypoint.sh /usr/local/bscp/docker-entrypoint.sh
COPY bscp /usr/local/bscp/bscp

RUN chmod +x /usr/local/bscp/bscp
RUN chmod +x /usr/local/bscp/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bscp/docker-entrypoint.sh"]
