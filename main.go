package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/amenzhinsky/consul-slack/consul"
	"github.com/amenzhinsky/consul-slack/slack"
)

func main() {
	var (
		interval = time.Second

		slackCfg = &slack.Config{
			Channel:  "consul",
			Username: "consul",
		}

		consulCfg = &consul.Config{
			Address:    "127.0.0.1:8500",
			Scheme:     "http",
			Datacenter: "dc1",
		}
	)

	flag.DurationVar(&interval, "interval", interval, "interval between consul api requests")
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
		fail(err)
	}

	s, err := slack.New(slackCfg)
	if err != nil {
		fail(err)
	}

	if err != nil {
		fail(err)
	}

	for {
		cc, pc, err := c.Next()
		if err != nil {
			fail(err)
		}

		for _, c := range cc {
			s.Danger("[%s] %s is critical", c.Node, c.ServiceName)
		}
		for _, c := range pc {
			s.Good("[%s] %s is passing", c.Node, c.ServiceName)
		}

		time.Sleep(interval)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
	os.Exit(1)
}
