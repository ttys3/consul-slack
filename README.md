# consul-slack

Consul services state slack notifier written in go.

## Usage

```
-consul-address string
      address of the consul server (default "127.0.0.1:8500")
-consul-datacenter string
      datacenter to use (default "dc1")
-consul-scheme string
      uri scheme of the consul server (default "http")
-interval duration
      interval between consul api requests (default 1s)
-slack-channel string
      slack channel name (default "#consul")
-slack-icon string
      slack user avatar url (default "https://www.consul.io/assets/images/logo_large-475cebb0.png")
-slack-url string
      slack webhook url [required]
-slack-username string
      slack user name (default "consul")
```
