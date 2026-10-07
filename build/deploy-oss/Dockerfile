# Future Gadget Laboratories OSS deploy image.
#
# Adapted from build/deploy/Dockerfile at CockroachDB v23.2.15.
# The base image is the public Red Hat UBI 9 minimal repository.
# Cockroach's private mirror (us-east1-docker.pkg.dev/crl-docker-sync/...)
# is not used.
#
# Build context (see build/fgdb/stage-image-context.sh):
#   cockroach.sh
#   cockroach          (the cockroach-oss binary, installed under this name)
#   libgeos.so
#   libgeos_c.so
#   licenses/
#
# This image must be built only from the OSS binary. Do not copy a
# //pkg/cmd/cockroach binary into this context.

FROM registry.access.redhat.com/ubi9/ubi-minimal

ARG fips_enabled=0
ARG image_version=v23.2.15-oss
ARG image_revision=unknown

LABEL org.opencontainers.image.title="FutureGadgetDatabases cockroach-oss" \
      org.opencontainers.image.description="CCL-free CockroachDB OSS binary built from v23.2.15" \
      org.opencontainers.image.source="https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${image_version}" \
      org.opencontainers.image.revision="${image_revision}"

# tzdata is reinstalled so /usr/share/zoneinfo is complete.
# hostname is used by Cockroach Kubernetes manifests.
# tar is used by kubectl cp.
RUN microdnf update -y \
    && rpm --erase --nodeps tzdata \
    && microdnf install tzdata hostname tar gzip xz -y \
    && rm -rf /var/cache/yum

# FIPS stays off unless the image is built with --build-arg fips_enabled=1.
# microdnf has no autoremove, so packages added only for fips-mode-setup
# are removed by diffing the rpm list.
RUN if [ "$fips_enabled" = "1" ]; then \
    microdnf install -y openssl && \
    rpm -qa | sort > /before.txt && \
    microdnf install -y crypto-policies-scripts && \
    fips-mode-setup --enable --no-bootcfg && \
    rpm -qa | sort > /after.txt && \
    microdnf remove -y $(comm -13 /before.txt /after.txt) && \
    microdnf clean all && \
    rm -rf /var/cache/yum /before.txt /after.txt; \
    fi

RUN mkdir -p /usr/local/lib/cockroach /cockroach /licenses /docker-entrypoint-initdb.d
COPY cockroach.sh cockroach /cockroach/
COPY licenses/ /licenses/
COPY libgeos.so libgeos_c.so /usr/local/lib/cockroach/

WORKDIR /cockroach/

ENV PATH=/cockroach:$PATH
ENV COCKROACH_CHANNEL=fgl-oss

EXPOSE 26257 8080
ENTRYPOINT ["/cockroach/cockroach.sh"]
