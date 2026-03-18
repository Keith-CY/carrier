FROM mcr.microsoft.com/playwright:v1.58.2-noble

ARG GO_VERSION=1.24.0

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl git jq openssh-client rsync unzip xz-utils \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
  && rm -rf /usr/local/go \
  && tar -C /usr/local -xzf /tmp/go.tgz \
  && rm -f /tmp/go.tgz

RUN curl -fsSL https://bun.sh/install | bash

ENV PATH="/root/.bun/bin:/usr/local/go/bin:${PATH}"
WORKDIR /workspace

ENTRYPOINT ["/workspace/harness/visual-acceptance/entrypoint.sh"]
