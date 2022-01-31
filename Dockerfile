FROM ubuntu:18.04 AS build-oneclient

RUN apt update && apt install -y --no-install-recommends python3 python3-setuptools python python-setuptools software-properties-common git dpkg-dev ca-certificates strace vim
RUN mkdir /build
WORKDIR /build

#RUN git clone -b release/20.02.15 https://github.com/onedata/oneclient.git
RUN git clone -b csi-oneclient-edit https://github.com/josefhandl/oneclient.git
#COPY oneclient oneclient
#COPY oneclient/.travis.yml .travis.yml

#RUN apt install -y software-properties-common
RUN add-apt-repository -y ppa:onedata/build-deps-2002
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Europe/Prague
RUN apt install -y --no-install-recommends $(gotit=n; \
while read line; do \
    [ $gotit = y -a "$line" = "" ] && break ;\
    set -- $line ;\
    [ "$gotit" = y ] && echo $2 ;\
    [ "$1" = packages: ] && gotit=y ;\
done < oneclient/.travis.yml )

WORKDIR /build/oneclient

RUN make submodules
RUN mkdir release
WORKDIR /build/oneclient/release
#RUN cmake --configure -DWITH_XROOTD=OFF ..

RUN gem install coveralls-lcov
RUN pip install six==1.12.0 dnspython Flask Flask-SQLAlchemy pytest==2.9.1 pytest-bdd==2.18.0 requests==2.5.1 boto boto3 rpyc==4.0.2 PyYAML xattr
RUN curl -L https://github.com/erlang/rebar3/releases/download/3.11.1/rebar3 -o /usr/local/bin/rebar3 && chmod +x /usr/local/bin/rebar3
RUN git config --global url."https://github.com/onedata".insteadOf "ssh://git@git.onedata.org:7999/vfs"

RUN export PKG_REVISION=$(git describe --tags --always --abbrev=7)
RUN export PKG_COMMIT=$(git rev-parse --verify HEAD)
RUN export HELPERS_COMMIT=$(git -C helpers rev-parse --verify HEAD)
#RUN cmake --configure -GNinja -DCMAKE_BUILD_TYPE=Release -DCODE_COVERAGE=ON -DWITH_CEPH=ON -DWITH_SWIFT=ON -DWITH_S3=ON -DWITH_GLUSTERFS=ON -DWITH_WEBDAV=ON -DWITH_XROOTD=OFF -DWITH_ONEDATAFS=ON ..
RUN cmake -GNinja -DCMAKE_BUILD_TYPE=Debug -DGIT_VERSION="20.02.15" -DCODE_COVERAGE=ON -DWITH_CEPH=ON -DWITH_SWIFT=ON -DWITH_S3=ON -DWITH_GLUSTERFS=ON -DWITH_WEBDAV=ON -DWITH_XROOTD=OFF -DWITH_ONEDATAFS=ON ..
WORKDIR /build/oneclient
RUN cmake --build release

RUN ls -la /build/oneclient/release

#========================================

#FROM golang:1.12-alpine3.9 AS  build-env
FROM ubuntu:20.04 AS build-driver
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

#========================================

#FROM alpine:3.9
FROM ubuntu:20.04

#RUN apk add --no-cache ca-certificates findmnt
RUN apt update && apt install -y --no-install-recommends ca-certificates curl


# Install oneclient - recommended (ubuntu 20.04 is not supported)
#RUN curl -sS  http://get.onedata.org/oneclient.sh | bash

# Install oneclient with fix for ubuntu 20.04
#ADD onedata/oneclient.sh /tmp/
#ADD onedata/onedata.gpg.key /tmp/
#RUN bash /tmp/oneclient.sh

# Install oneclient with fsstat fix - builded in previous steps
COPY --from=build-oneclient /build/client/release /opt/oneclient
RUN ln -s /opt/oneclient/oneclient /bin/oneclient

ADD onedata/mount.onedata /sbin/mount.onedata
RUN chmod +x /sbin/mount.onedata

COPY --from=build-driver /csi-sshfs /bin/csi-sshfs

ADD onedata/wrapper.sh /tmp/
RUN chmod +x /tmp/wrapper.sh

ENTRYPOINT ["/tmp/wrapper.sh"]
CMD [""]
#ENTRYPOINT ["exit", "0"]
#ENTRYPOINT ["sleep", "9999"]
