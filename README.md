# consul-slack [![CircleCI](https://circleci.com/gh/amenzhinsky/consul-slack.svg?style=svg)](https://circleci.com/gh/amenzhinsky/consul-slack)

Consul services state slack notifier written in go.

## Running

You can safely run multiple consul-slack instances because they use locking strategy based on the consul KV.

### Systemd
```
[Unit]
Description=Consul slack notifier
Wants=network.target

[Service]
Type=simple
User=consul-slack
Group=consul-slack
ExecStart=/usr/local/bin/consul-slack WEBHOOK_URL \
  -slack-channel '#consul' \
  -slack-username Consul \
  -slack-icon https://image-url \
  -consul-address 127.0.0.1:8500 \
  -consul-schema http \
  -consul-datacenter dc1
Restart=on-failure

[Install]
WantedBy=multi-user.target
```
