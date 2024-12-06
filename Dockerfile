
FROM ubuntu:22.04 AS build-driver

RUN apt update && apt install -y --no-install-recommends \
        git \
        golang \
        ca-certificates \
        apt-utils

ENV CGO_ENABLED=0, GO111MODULE=on
WORKDIR /go/src/csi-onedata

ADD . /go/src/csi-onedata

RUN go mod download

SHELL ["/bin/bash", "-c"]
RUN export BUILD_TIME=`date -R` && \
    export VERSION=`cat /go/src/csi-onedata/version.txt` && echo "time $BUILD_TIME version $VERSION" && \
    go build -o /csi-onedata -ldflags "-X 'csi-onedata/pkg/oneclient.BuildTime=${BUILD_TIME}' -X 'csi-onedata/pkg/oneclient.Version=${VERSION}'" csi-onedata/cmd/csi-onedata

#========================================

FROM ubuntu:22.04

ENV url='http://packages.onedata.org'
ENV package='oneclient=21.02.3-1~jammy'

RUN apt update && apt install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg

# Install oneclient
#RUN curl -sS  http://get.onedata.org/oneclient.sh | bash

# Install oneclient manually to select specific version
RUN curl ${url}/onedata.gpg.key | apt-key add - \
    && echo "deb [arch=amd64] ${url}/apt/ubuntu/2102 jammy main" > /etc/apt/sources.list.d/onedata.list \
    && echo "deb-src [arch=amd64] ${url}/apt/ubuntu/2102 jammy main" >> /etc/apt/sources.list.d/onedata.list \
    && apt-get update \
    && apt-get install -y ${package}

# Add mount wrapper
ADD onedata/mount.onedata /sbin/mount.onedata
RUN chmod +x /sbin/mount.onedata

# Copy csi driver
COPY --from=build-driver /csi-onedata /bin/csi-onedata

ADD onedata/wrapper.sh /tmp/
RUN chmod +x /tmp/wrapper.sh

ENTRYPOINT ["/tmp/wrapper.sh"]
#ENTRYPOINT ["/bin/csi-onedata"]
CMD [""]

