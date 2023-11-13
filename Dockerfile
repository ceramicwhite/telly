# syntax=docker/dockerfile:1.6-labs

ARG TELLY_VERSION=v1.5.0
ARG S6_OVERLAY_VERSION=v3.1.6.0

# First stage: build the Go package
FROM --platform=$BUILDPLATFORM golang:1.10.8-stretch as builder

ARG TARGETPLATFORM
ARG TELLY_VERSION

RUN echo "deb http://archive.debian.org/debian-security stretch/updates main" > /etc/apt/sources.list

# Install dependencies and tools needed to build `dep` from source if not on amd64
RUN if [ "$TARGETPLATFORM" != "linux/amd64" ]; then \
        apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev git && \
        go get -d -v github.com/golang/dep/cmd/dep && \
        cd $(go env GOPATH)/src/github.com/golang/dep/cmd/dep && \
        go build -o /usr/bin/dep; \
    else \
         wget --output-document /usr/bin/dep https://github.com/golang/dep/releases/download/v0.5.0/dep-linux-amd64 && \
        chmod +x /usr/bin/dep; \
    fi

RUN chmod +x /usr/bin/dep

# Set up the working directory in GOPATH
WORKDIR $GOPATH/src/github.com/tellytv/telly

# Clone the specific branch of the repository
#RUN git clone 
ADD https://github.com/tellytv/telly.git#${TELLY_VERSION} .

# Install dependencies with dep
RUN dep ensure --vendor-only

# Build the Go package
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /telly .

# Second stage: setup the runtime environment
FROM debian:bookworm-slim

ARG S6_OVERLAY_VERSION

ENV DEBIAN_FRONTEND=noninteractive \
    LANG='C.UTF-8' \
    LANGUAGE='C.UTF-8' \
    LC_ALL='C.UTF-8'

# Copy static ffmpeg binaries from the builder stage
COPY --from=ceramicwhite/static-ffmpeg /ffmpeg /ffprobe /usr/bin/

# Copy the built telly binary from the builder stage
COPY --from=builder /telly /usr/bin/telly

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y \
        wget \
        xz-utils \
        tzdata && \
    ARCH=$(uname -m) && \
        case "$ARCH" in \
            x86_64) FILENAME="s6-overlay-x86_64.tar.xz" ;; \
            aarch64) FILENAME="s6-overlay-aarch64.tar.xz" ;; \
            *) echo "Unsupported architecture: $ARCH" && exit 1 ;; \
        esac && \
        S6_OVERLAY_URL="https://github.com/just-containers/s6-overlay/releases/download/${S6_OVERLAY_VERSION}/${FILENAME}" && \
        S6_OVERLAY_SHA_URL="${S6_OVERLAY_URL}.sha256" && \
        wget $S6_OVERLAY_SHA_URL -O /tmp/${FILENAME}.sha256 && \
        wget $S6_OVERLAY_URL -O /tmp/${FILENAME} && \
        cd /tmp && \
        sha256sum -c ${FILENAME}.sha256 && \
        tar -xJf ${FILENAME} -C / 

COPY s6/ /etc

EXPOSE 6077

VOLUME /config

ENTRYPOINT ["/init"]
