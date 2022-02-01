FROM ubuntu:18.04 AS build-oneclient

RUN apt update && apt install -y --no-install-recommends \
    python3 \
    python3-setuptools \
    python \
    python-setuptools \
    software-properties-common \
    git \
    dpkg-dev \
    ca-certificates

RUN mkdir /build
WORKDIR /build

#RUN git clone -b release/20.02.15 https://github.com/onedata/oneclient.git
RUN git clone -b csi-oneclient-edit https://github.com/josefhandl/oneclient.git
#COPY oneclient oneclient

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

FROM ubuntu:20.04 AS build-driver
RUN apt update && apt install -y --no-install-recommends git golang ca-certificates apt-utils

ENV CGO_ENABLED=0, GO111MODULE=on
WORKDIR /go/src/csi-onedata

ADD . /go/src/csi-onedata

RUN go mod download

SHELL ["/bin/bash", "-c"]
RUN export BUILD_TIME=`date -R` && \
    export VERSION=`cat /go/src/csi-onedata/version.txt` && echo "time $BUILD_TIME version $VERSION" && \
    go build -o /csi-onedata -ldflags "-X 'csi-onedata/pkg/oneclient.BuildTime=${BUILD_TIME}' -X 'csi-onedata/pkg/oneclient.Version=${VERSION}'" csi-onedata/cmd/csi-onedata

#========================================

FROM ubuntu:18.04

RUN apt update && apt install -y --no-install-recommends \
    ca-certificates \
    curl

# Required by the oneclient
RUN apt install -y --no-install-recommends \
    libgoogle-glog0v5 \
    fuse \
    libboost-context1.65.1 \
    libboost-filesystem1.65.1 \
    libboost-iostreams1.65.1 \
    libboost-log1.65.1 \
    libboost-program-options1.65.1 \
    libboost-python1.65.1 \
    libboost-random1.65.1 \
    libboost-system1.65.1 \
    libboost-thread1.65.1 \
    libevent-2.1-6 \
    libdouble-conversion1 \
    libtbb2 \
    libprotobuf10 \
    libradosstriper1 \
    libpoconetssl50 \
    glusterfs-common \
    libsodium23

# Install oneclient - recommended (ubuntu 20.04 is not supported)
#RUN curl -sS  http://get.onedata.org/oneclient.sh | bash

# Install oneclient with fix for ubuntu 20.04
#ADD onedata/oneclient.sh /tmp/
#ADD onedata/onedata.gpg.key /tmp/
#RUN bash /tmp/oneclient.sh

# Install oneclient with fsstat fix - builded in previous steps
COPY --from=build-oneclient /build/oneclient /opt/oneclient
RUN ln -s /opt/oneclient/release/oneclient /usr/bin/oneclient

ADD onedata/mount.onedata /sbin/mount.onedata
RUN chmod +x /sbin/mount.onedata

COPY --from=build-driver /csi-onedata /bin/csi-data

ADD onedata/wrapper.sh /tmp/
RUN chmod +x /tmp/wrapper.sh

ENTRYPOINT ["/tmp/wrapper.sh"]
CMD [""]
