
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
RUN curl -sS  http://get.onedata.org/oneclient.sh | bash

# Install oneclient with fix for ubuntu 20.04
ADD onedata/oneclient.sh /tmp/
ADD onedata/onedata.gpg.key /tmp/
RUN bash /tmp/oneclient.sh

# Install oneclient with fsstat fix - builded in previous steps
#COPY --from=build-oneclient /build/oneclient /opt/oneclient
#RUN ln -s /opt/oneclient/release/oneclient /usr/bin/oneclient

ADD onedata/mount.onedata /sbin/mount.onedata
RUN chmod +x /sbin/mount.onedata

COPY --from=build-driver /csi-onedata /bin/csi-onedata

ADD onedata/wrapper.sh /tmp/
RUN chmod +x /tmp/wrapper.sh

ENTRYPOINT ["/tmp/wrapper.sh"]
#ENTRYPOINT ["/bin/csi-onedata"]
CMD [""]
