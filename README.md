# consul-slack [![CircleCI](https://circleci.com/gh/amenzhinsky/consul-slack.svg?style=svg)](https://circleci.com/gh/amenzhinsky/consul-slack)

Consul services state slack notifier written in go.

## Running

As a systemd service:
```
[Unit]
Description=Consul slack notifier
Wants=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/consul-slack \
	-slack-url WEBHOOK_URL \
	-slack-channel '#consul' \
	-slack-username Consul \
	-consul-address 127.0.0.1:8500

[Install]
WantedBy=multi-user.target
```
