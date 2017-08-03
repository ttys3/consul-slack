# consul-slack [![CircleCI](https://circleci.com/gh/amenzhinsky/consul-slack.svg?style=svg)](https://circleci.com/gh/amenzhinsky/consul-slack)

Consul services state slack notifier written in go.

## Running

If you're running multiple instances use the `consul lock` because mutual access to the consul KV storage may lead to unexpected behaviour.

### Systemd
```
[Unit]
Description=Consul slack notifier
Wants=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/consul-slack WEBHOOK_URL \
	-slack-channel '#consul' \
	-slack-username Consul \
	-slack-icon https://image-url \
	-consul-address 127.0.0.1:8500 \
	-consul-schema http \
	-consul-datacenter dc1

[Install]
WantedBy=multi-user.target
```
