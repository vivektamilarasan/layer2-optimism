FROM --platform=linux/amd64 us-docker.pkg.dev/oplabs-tools-artifacts/images/ci-builder:v0.51.0 as ci-builder

FROM --platform=linux/amd64 debian:bookworm-slim as deployer-builder

WORKDIR /workspace

# Install dependencies using apt
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    make \
    jq \
    direnv \
    bash \
    curl \
    build-essential \
    ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ci-builder /usr/local/bin/svm /usr/local/bin/svm
COPY --from=ci-builder /usr/local/bin/forge /usr/local/bin/forge
COPY --from=ci-builder /usr/local/bin/cast /usr/local/bin/cast
COPY --from=ci-builder /usr/local/bin/anvil /usr/local/bin/anvil
COPY --from=ci-builder /usr/local/bin/just /usr/local/bin/just

RUN svm install 0.8.25 && \
  svm install 0.8.15 && \
  svm install 0.8.19

RUN mkdir -p /workspace/optimism/packages/contracts-bedrock

WORKDIR /workspace/optimism/packages/contracts-bedrock

COPY lib ./lib
COPY scripts ./scripts
COPY src ./src
COPY test ./test
COPY deploy.sh ./deploy.sh
COPY foundry.toml ./foundry.toml
COPY justfile ./justfile
COPY safe.json ./safe.json
COPY semver-lock.json ./semver-lock.json
COPY LICENSE ./LICENSE

RUN bash scripts/ops/pull-artifacts.sh

CMD ["bash", "/workspace/deploy.sh"]