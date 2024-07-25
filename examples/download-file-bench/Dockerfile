FROM alpine:3.18.3

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk --update --no-cache add ca-certificates bash vim curl

WORKDIR /bscp

COPY download-file-bench ./

ENTRYPOINT ["/bscp/download-file-bench"]
CMD []
