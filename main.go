package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/ttys3/consul-slack/consul"
	"github.com/ttys3/consul-slack/slack"
)

var (
	ServiceName = ""
	Version     = "dev"
	BuildTime   = ""
)

var (
	slackUsernameFlag = "Consul"
	slackIconURLFlag  = "https://www.consul.io/assets/images/logo_large-475cebb0.png"

	consulAddressFlag    = ""
	consulSchemeFlag     = ""
	consulDatacenterFlag = "dc1"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s SLACK_WEBHOOK_URL\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.StringVar(&slackUsernameFlag, "slack-username", slackUsernameFlag, "slack user name")
	flag.StringVar(&slackIconURLFlag, "slack-icon", slackIconURLFlag, "slack user avatar url")
	flag.StringVar(&consulAddressFlag, "consul-address", consulAddressFlag, "address of the consul server, default to 127.0.0.1:8500")
	flag.StringVar(&consulSchemeFlag, "consul-scheme", consulSchemeFlag, "uri scheme of the consul server, default to http")
	flag.StringVar(&consulDatacenterFlag, "consul-datacenter", consulDatacenterFlag, "datacenter to use, default to dc1")
	flag.Parse()

	slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if flag.NArg() == 1 {
		slackWebhookURL = flag.Arg(0)
	}

	if slackWebhookURL == "" {
		log.Println("error: empty SLACK_WEBHOOK_URL")
		flag.Usage()
		os.Exit(1)
	}

	log.Println(ServiceName, Version, BuildTime)

	if err := start(slackWebhookURL); err != nil {
		fmt.Fprintf(os.Stderr, "exited with error: %v\n", err)
		os.Exit(1)
	}
}

func start(webhookURL string) error {
	s, err := slack.New(webhookURL,
		slack.WithUsername(slackUsernameFlag),
		slack.WithIconURL(slackIconURLFlag),
	)
	if err != nil {
		return err
	}

	c, err := consul.New(
		consul.WithAddress(consulAddressFlag),
		consul.WithDatacenter(consulDatacenterFlag),
		consul.WithScheme(consulSchemeFlag),
	)
	if err != nil {
		return err
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		if err := c.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close error: %v", err)
		}
	}()

	go healthCheck()

	for ev := c.Next(); ev != nil; ev = c.Next() {
		switch ev.Status {
		case consul.Passing:
			s.Good("[%s] %s is back to normal\nNotes: %s\nOutput: %s", ev.Node, ev.ServiceID, ev.Notes, ev.Output)
		case consul.Warning:
			s.Warning("[%s] %s is having problems\nNotes: %s\nOutput: %s", ev.Node, ev.ServiceID, ev.Notes, ev.Output)
		case consul.Critical:
			s.Danger("[%s] %s is critical\nNotes: %s\nOutput: %s", ev.Node, ev.ServiceID, ev.Notes, ev.Output)
		case consul.Maintenance:
			s.Message("[%s] %s is under maintenance\nNotes: %s", ev.Node, ev.ServiceID, ev.Notes)
		default:
			panic(fmt.Sprintf("unknown status %q", ev.Status))
		}
	}
	return c.Err()
}

func healthCheck() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	listenAddr := ":8080"
	if addr := os.Getenv("HEALTH_CHECK_ADDR"); addr != "" {
		listenAddr = addr
	}

	log.Printf("start health check http service on %v\n", listenAddr)
	http.ListenAndServe(listenAddr, nil)
}
