# Future Gadget Laboratories OSS deploy image.
#
# Adapted from build/deploy/Dockerfile at CockroachDB v23.2.15.
# The base image is the public Red Hat UBI 9 minimal repository.
# Cockroach's private mirror (us-east1-docker.pkg.dev/crl-docker-sync/...)
# is not used.
#
# Pin by digest only. Do not put a tag on the FROM line, and do not use
# the floating tags "latest" or "9.8"; those move when Red Hat publishes
# again. Checked 2026-10-07: this digest is UBI 9 minimal 9.8-1791279563.
# The floating tags "9.8" and "latest" pointed at it that day.
# To bump: list tags at
#   https://registry.access.redhat.com/v2/ubi9/ubi-minimal/tags/list
# pick the newest 9.<minor>-<build> tag, request that tag's manifest list
# (Accept: application/vnd.docker.distribution.manifest.list.v2+json),
# and replace only the sha256 below with the Docker-Content-Digest header.
# Write the human-readable tag in this comment, not on the FROM line.
# The sha256 is the manifest list, not the linux/amd64 blob.
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
#
# The process runs as root. That is the same choice as the upstream
# CockroachDB image. Charts and existing volumes expect the database
# files under /cockroach/cockroach-data to be owned by root. To run as
# another user, use a volume that user can already write, and set the
# pod securityContext runAsUser and runAsGroup to that user's ids. This
# image does not change ownership of the data directory on startup.

FROM registry.access.redhat.com/ubi9/ubi-minimal@sha256:5ed244b62bbf4095080144d9d35eb8fcd3d39a9801f94aadd63b9d10978a01ae

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
# FIPS stays off unless the image is built with --build-arg fips_enabled=1.
# microdnf has no autoremove, so packages added only for fips-mode-setup
# are removed by diffing the rpm list.
RUN microdnf update -y \
    && rpm --erase --nodeps tzdata \
    && microdnf install tzdata hostname tar gzip xz -y \
    && if [ "$fips_enabled" = "1" ]; then \
         microdnf install -y openssl \
         && rpm -qa | sort > /before.txt \
         && microdnf install -y crypto-policies-scripts \
         && fips-mode-setup --enable --no-bootcfg \
         && rpm -qa | sort > /after.txt \
         && microdnf remove -y $(comm -13 /before.txt /after.txt) \
         && microdnf clean all \
         && rm -rf /before.txt /after.txt; \
       fi \
    && rm -rf /var/cache/yum \
    && mkdir -p /usr/local/lib/cockroach /cockroach /licenses /docker-entrypoint-initdb.d
COPY cockroach.sh cockroach /cockroach/
COPY licenses/ /licenses/
COPY libgeos.so libgeos_c.so /usr/local/lib/cockroach/

WORKDIR /cockroach/

ENV PATH=/cockroach:$PATH
ENV COCKROACH_CHANNEL=fgl-oss

EXPOSE 26257 8080
ENTRYPOINT ["/cockroach/cockroach.sh"]
