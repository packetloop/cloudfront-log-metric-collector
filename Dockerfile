FROM arbornetworks-docker-v2.bintray.io/aws-cli_0.2.0:18da34d

ARG GITHUB_OWNER=packetloop
ARG GITHUB_REPO=cloudfront-metrics-collector
ARG GITHUB_TAG
ARG GITHUB_ASSET_FILENAME
ARG GITHUB_TOKEN
ARG PROJECT_DIR=/opt/cloudfront-metrics-collector

ENV SCRIPT_PATH=${PROJECT_DIR}

ENV CLUB_NAME=""
ENV SQS_QUEUE_URL=""
ENV SQS_REGION=us-west-2
ENV STATSD_HOST=localhost:8125
ENV STATSD_NETWORK=udp
ENV STATSD_PREFIX="collector"
ENV EXECUTABLE=${GITHUB_ASSET_FILENAME}

RUN mkdir -p ${SCRIPT_PATH}
COPY Makefile ${SCRIPT_PATH}

RUN GITHUB_ASSET_ID=$(curl -sSL https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/tags/${GITHUB_TAG}?access_token=${GITHUB_TOKEN} | \
  grep -B 1 "\"name\": \"${GITHUB_ASSET_FILENAME}\"" | head -1 | sed 's/.*"id": \(.*\),/\1/') && \
  curl -L -o ${SCRIPT_PATH}/${GITHUB_ASSET_FILENAME} \
  -H 'Accept: application/octet-stream' \
  https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/assets/${GITHUB_ASSET_ID}?access_token=${GITHUB_TOKEN} && \
  chmod +x ${SCRIPT_PATH}/${GITHUB_ASSET_FILENAME}

WORKDIR ${SCRIPT_PATH}

CMD [ "make", "run"]
