FROM --platform=linux/amd64 golang:1.23.0-bookworm as go-builder

COPY ./go.mod /app/go.mod
COPY ./go.sum /app/go.sum
COPY ./op-node /app/op-node
COPY ./op-service /app/op-service
COPY ./op-alt-da /app/op-alt-da
COPY ./op-chain-ops /app/op-chain-ops

WORKDIR /app/op-chain-ops

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go mod download

RUN go build -o ./bin/op-deployer ./cmd/op-deployer/main.go

FROM --platform=linux/amd64 us-docker.pkg.dev/oplabs-tools-artifacts/images/ci-builder:v0.51.0 as ci-builder

FROM --platform=linux/amd64 debian:bookworm-slim

WORKDIR /workspace

# Install dependencies using apt
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    jq \
    bash \
    curl \
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

RUN mkdir deployments && \
    mkdir deploy-config

COPY packages/contracts-bedrock/lib ./lib
COPY packages/contracts-bedrock/scripts ./scripts
COPY packages/contracts-bedrock/src ./src
COPY packages/contracts-bedrock/test ./test
COPY packages/contracts-bedrock/foundry.toml ./foundry.toml
COPY packages/contracts-bedrock/justfile ./justfile
COPY packages/contracts-bedrock/safe.json ./safe.json
COPY packages/contracts-bedrock/semver-lock.json ./semver-lock.json
COPY packages/contracts-bedrock/LICENSE ./LICENSE
COPY packages/contracts-bedrock/artifacts ./artifacts
COPY packages/contracts-bedrock/cache ./cache
COPY packages/contracts-bedrock/forge-artifacts ./forge-artifacts

WORKDIR /workspace/optimism

COPY ops/docker/op-deployer/deploy.sh /usr/local/bin/deploy.sh

COPY --from=go-builder /app/op-chain-ops/bin/op-deployer /usr/local/bin/op-deployer

CMD ["op-deployer", "--local"]