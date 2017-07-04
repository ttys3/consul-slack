package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/amenzhinsky/consul-slack/consul"
	"github.com/amenzhinsky/consul-slack/slack"
)

// default slack client configuration.
var slackCfg = &slack.Config{
	Channel:  "#consul",
	Username: "Consul",
	IconURL:  "https://www.consul.io/assets/images/logo_large-475cebb0.png",
}

// default consul client configuration.
var consulCfg = &consul.Config{
	Address:    "127.0.0.1:8500",
	Scheme:     "http",
	Datacenter: "dc1",
}

func main() {
	flag.StringVar(&slackCfg.WebhookURL, "slack-url", slackCfg.WebhookURL, "slack webhook url [required]")
	flag.StringVar(&slackCfg.Channel, "slack-channel", slackCfg.Channel, "slack channel name")
	flag.StringVar(&slackCfg.Username, "slack-username", slackCfg.Username, "slack user name")
	flag.StringVar(&slackCfg.IconURL, "slack-icon", slackCfg.IconURL, "slack user avatar url")
	flag.StringVar(&consulCfg.Address, "consul-address", consulCfg.Address, "address of the consul server")
	flag.StringVar(&consulCfg.Scheme, "consul-scheme", consulCfg.Scheme, "uri scheme of the consul server")
	flag.StringVar(&consulCfg.Datacenter, "consul-datacenter", consulCfg.Datacenter, "datacenter to use")
	flag.BoolVar(&consulCfg.Debug, "debug", consulCfg.Debug, "enable debug mode")
	flag.Parse()

	if slackCfg.WebhookURL == "" {
		fmt.Fprintln(os.Stderr, "-slack-url is empty")
		os.Exit(1)
	}

	if err := start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func start() error {
	c, err := consul.New(consulCfg)
	if err != nil {
		return err
	}

	s, err := slack.New(slackCfg)
	if err != nil {
		return err
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-ch
		c.Close()
	}()

	for ev := range c.C {
		if ev.Err != nil {
			return err
		}

		switch ev.Status {
		case consul.Critical:
			s.Danger("[%s] %s service is critical", ev.Node, ev.ServiceID)
		case consul.Passing:
			s.Good("[%s] %s service is back to normal", ev.Node, ev.ServiceID)
		case consul.Warning:
			s.Warning("[%s] %s is having problems", ev.Node, ev.ServiceID)
		}
	}
	return nil
}
