package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// New creates new slack client
func New(cfg *Config) (*Slack, error) {
	if cfg == nil {
		panic("cfg is nil")
	}

	return &Slack{cfg: cfg}, nil
}

// Config is slack config
type Config struct {
	// WebhookURL is webhook url
	WebhookURL string

	// Chanel is slack channel name, starts with #
	Channel string

	// Username of user that messages are sent on behalf of
	Username string

	// IconURL is avatar url
	IconURL string
}

// Slack is slack client
type Slack struct {
	cfg *Config
}

// payload is data that is sent to webhook url
type payload struct {
	Channel     string       `json:"channel"`
	Username    string       `json:"username"`
	IconURL     string       `json:"icon_url"`
	Attachments []attachment `json:"attachments"`
}

type attachment struct {
	Color string `json:"color"`
	Text  string `json:"text"`
}

// Critical sends a message that service is critical
func (s *Slack) Critical(node, service string) error {
	return s.notify("danger", "critical", node, service)
}

// Passing sends a message that service is passing
func (s *Slack) Passing(node, service string) error {
	return s.notify("good", "passing", node, service)
}

// notify sends message to webhook url
func (s *Slack) notify(color, state, node, service string) error {
	if s.cfg.WebhookURL == "" {
		return nil
	}

	b, err := json.Marshal(&payload{
		Channel:  s.cfg.Channel,
		Username: s.cfg.Username,
		IconURL:  s.cfg.IconURL,
		Attachments: []attachment{
			{
				Color: color,
				Text:  fmt.Sprintf("[%s] %s is %s now", node, service, state),
			},
		},
	})

	if err != nil {
		return err
	}

	_, err = http.Post(s.cfg.WebhookURL, "application/json", bytes.NewReader(b))
	return err
}
