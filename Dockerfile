FROM alpine:3.2
RUN apk update && apk add --no-cache ca-certificates
ADD . /app
RUN mkdir -p /var/repos
RUN mkdir -p /var/repos/build

ADD hooks_executor.go /var/repos/hooks_executors.go
WORKDIR /app
ENTRYPOINT [ "/app/paas" ]