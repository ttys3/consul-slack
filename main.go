package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/amenzhinsky/consul-slack/consul"
	"github.com/amenzhinsky/consul-slack/slack"
)

// exitErr is last error occurred before the main function returns
var exitErr error

func main() {
	// make sure that all defers are executed before program exits
	defer func() {
		fail(exitErr)
	}()

	var (
		slackCfg = &slack.Config{
			Channel:  "#consul",
			Username: "Consul",
			IconURL:  "https://www.consul.io/assets/images/logo_large-475cebb0.png",
		}

		consulCfg = &consul.Config{
			Address:    "127.0.0.1:8500",
			Scheme:     "http",
			Datacenter: "dc1",
		}
	)

	flag.StringVar(&slackCfg.WebhookURL, "slack-url", slackCfg.WebhookURL, "slack webhook url [required]")
	flag.StringVar(&slackCfg.Channel, "slack-channel", slackCfg.Channel, "slack channel name")
	flag.StringVar(&slackCfg.Username, "slack-username", slackCfg.Username, "slack user name")
	flag.StringVar(&slackCfg.IconURL, "slack-icon", slackCfg.IconURL, "slack user avatar url")
	flag.StringVar(&consulCfg.Address, "consul-address", consulCfg.Address, "address of the consul server")
	flag.StringVar(&consulCfg.Scheme, "consul-scheme", consulCfg.Scheme, "uri scheme of the consul server")
	flag.StringVar(&consulCfg.Datacenter, "consul-datacenter", consulCfg.Datacenter, "datacenter to use")
	flag.Parse()

	if slackCfg.WebhookURL == "" {
		fail(errors.New("slack-url is empty"))
	}

	c, err := consul.New(consulCfg)
	if err != nil {
		exitErr = err
		return
	}

	s, err := slack.New(slackCfg)
	if err != nil {
		exitErr = err
		return
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-ch
		c.Close()
	}()

	for {
		checks, err := c.Next()
		if err != nil {
			if err == consul.ErrClosed {
				return
			}

			exitErr = err
			return
		}

		for _, c := range checks {
			switch c.Status {
			case "critical":
				s.Danger("[%s] %s service is critical", c.Node, c.ServiceName)
			case "passing":
				s.Good("[%s] %s service is back to normal", c.Node, c.ServiceName)
			case "warning":
				s.Warning("[%s] %s is having problems", c.Node, c.ServiceName)
			}
		}
	}
}

func fail(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
	os.Exit(1)
}
