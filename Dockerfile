#FROM golang:1.12-alpine3.9 AS  build-env
FROM ubuntu:20.04 AS build-env
#RUN apk add --no-cache git
RUN apt update && apt install -y --no-install-recommends git golang ca-certificates apt-utils
#RUN apt update && apt install -y git golang
#RUN apt search golang | grep installed

ENV CGO_ENABLED=0, GO111MODULE=on
WORKDIR /go/src/github.com/chr-fritz/csi-sshfs

ADD . /go/src/github.com/chr-fritz/csi-sshfs

RUN go mod download

SHELL ["/bin/bash", "-c"]
RUN export BUILD_TIME=`date -R` && \
    export VERSION=`cat /go/src/github.com/chr-fritz/csi-sshfs/version.txt` && echo "time $BUILD_TIME version $VERSION" && \
    go build -o /csi-sshfs -ldflags "-X 'github.com/chr-fritz/csi-sshfs/pkg/sshfs.BuildTime=${BUILD_TIME}' -X 'github.com/chr-fritz/csi-sshfs/pkg/sshfs.Version=${VERSION}'" github.com/chr-fritz/csi-sshfs/cmd/csi-sshfs

#FROM alpine:3.9
FROM ubuntu:20.04

#RUN apk add --no-cache ca-certificates findmnt
RUN apt update && apt install -y --no-install-recommends ca-certificates curl


# Install oneclient (OneData client)
#RUN curl -sS  http://get.onedata.org/oneclient.sh | bash
ADD onedata/oneclient.sh /tmp/
ADD onedata/onedata.gpg.key /tmp/
RUN bash /tmp/oneclient.sh
ADD onedata/mount.onedata /sbin/mount.onedata
RUN chmod +x /sbin/mount.onedata

COPY --from=build-env /csi-sshfs /bin/csi-sshfs

ADD onedata/wrapper.sh /tmp/
RUN chmod +x /tmp/wrapper.sh

ENTRYPOINT ["/tmp/wrapper.sh"]
CMD [""]
#ENTRYPOINT ["exit", "0"]
#ENTRYPOINT ["sleep", "9999"]
