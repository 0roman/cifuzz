# This image contains the dependencies to run C/C++ fuzz tests with cifuzz.
# It is uploaded to https://hub.docker.com/r/cifuzz/cifuzz-ubuntu and used
# as default image for the `cifuzz container` commands (and bundling)

FROM ubuntu:rolling

# Install APT packages with caching set up according to
# https://docs.docker.com/engine/reference/builder/#example-cache-apt-packages
RUN rm -f /etc/apt/apt.conf.d/docker-clean && \
    echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
  --mount=type=cache,target=/var/lib/apt,sharing=locked \
    export DEBIAN_FRONTEND=noninteractive && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    llvm \
    && rm -rf /var/lib/apt/lists/*
