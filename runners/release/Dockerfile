FROM ubuntu:24.04@sha256:24737cef17f4e22b1977df7d9a05e77c4e28b7da5b8911f8d8ccda136bf3573c
ARG RUNNER_VERSION=2.338.2
ARG RUNNER_SHA256=baebf204307bac6193480852732f0aba2ae97b6c5597c244f4d5558390147c40
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git jq python3 unzip gh sudo libicu74 libssl3t64 libkrb5-3 zlib1g && rm -rf /var/lib/apt/lists/*
ARG AWSCLI_VERSION=2.35.14
ARG AWSCLI_SHA256=e8124b5b1cfdf69ec97d95f1b1b1e331afaf4f752b5bca153fa2b354785880f3
RUN curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64-${AWSCLI_VERSION}.zip" -o /tmp/awscliv2.zip && echo "${AWSCLI_SHA256}  /tmp/awscliv2.zip" | sha256sum -c - && unzip -q /tmp/awscliv2.zip -d /tmp && /tmp/aws/install && rm -rf /tmp/aws /tmp/awscliv2.zip
RUN useradd --create-home runner && mkdir /opt/runner && chown runner:runner /opt/runner
WORKDIR /opt/runner
RUN curl -fsSL "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" -o /tmp/runner.tar.gz && echo "${RUNNER_SHA256}  /tmp/runner.tar.gz" | sha256sum -c - && tar xzf /tmp/runner.tar.gz && rm /tmp/runner.tar.gz && chown -R runner:runner /opt/runner
COPY --chown=runner:runner runners/release/entrypoint.sh /opt/runner/entrypoint.sh
USER runner
ENTRYPOINT ["bash", "/opt/runner/entrypoint.sh"]
