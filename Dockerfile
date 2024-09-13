FROM ubuntu:24.04

COPY consul-slack  /usr/local/bin/

RUN mkdir /etc/consul-slack; \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates tzdata; \
    update-ca-certificates -f; \
    apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false; \
    apt autoremove -y; \
    rm -rf /var/lib/apt/lists/*

# Add Tini
ENV TINI_VERSION v0.19.0
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini
RUN chmod +x /tini

WORKDIR /usr/local/bin/

# github.com/hashicorp/consul/api@v1.12.0/api.go
ENV TZ=Asia/Shanghai \
    HEALTH_CHECK_ADDR=:8080 \
    SLACK_WEBHOOK_URL="" \
    CONSUL_HTTP_ADDR="" \
    CONSUL_HTTP_SSL=false \
    CONSUL_CACERT="" \
    CONSUL_CLIENT_KEY="" \
    CONSUL_CLIENT_CERT="" \
    CONSUL_TLS_SERVER_NAME="" \
    CONSUL_HTTP_SSL_VERIFY=false

ENTRYPOINT ["/tini", "--"]
# Run your program under Tini
CMD ["/usr/local/bin/consul-slack"]
