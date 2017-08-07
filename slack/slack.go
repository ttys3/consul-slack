package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Option is a configuration value.
type Option func(s *Slack)

// WithChannel sets channel name.
func WithChannel(channel string) Option {
	return func(s *Slack) {
		s.channel = channel
	}
}

// WithUsername sets username that messages are sent on behalf of.
func WithUsername(username string) Option {
	return func(s *Slack) {
		s.username = username
	}
}

// WithIconURL sets icon url.
func WithIconURL(url string) Option {
	return func(s *Slack) {
		s.iconURL = url
	}
}

// WithLogger sets logger.
func WithLogger(l *log.Logger) Option {
	return func(s *Slack) {
		s.logger = l
	}
}

// New creates new slack client.
func New(url string, opts ...Option) (*Slack, error) {
	s := &Slack{
		webhookURL: url,
		username:   "webhooker",
		channel:    "webhooks",
		logger:     log.New(os.Stdout, "[slack] ", log.LstdFlags),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Slack is a slack client.
type Slack struct {
	webhookURL string
	channel    string
	username   string
	iconURL    string
	logger     *log.Logger
}

// payload is data that is sent to the webhook url.
type payload struct {
	Channel     string       `json:"channel"`
	Username    string       `json:"username"`
	IconURL     string       `json:"icon_url"`
	Attachments []attachment `json:"attachments"`
}

// attachment is a message container.
type attachment struct {
	Color string `json:"color"`
	Text  string `json:"text"`
}

// Danger is equivalent of Send("danger", ...)
func (s *Slack) Danger(msg string, v ...interface{}) error {
	return s.Send("danger", msg, v...)
}

// Good is equivalent of Send("good", ...)
func (s *Slack) Good(msg string, v ...interface{}) error {
	return s.Send("good", msg, v...)
}

// Warning is equivalent of Send("warning", ...)
func (s *Slack) Warning(msg string, v ...interface{}) error {
	return s.Send("warning", msg, v...)
}

// Send sends message to the webhook url.
func (s *Slack) Send(color, msg string, v ...interface{}) error {
	b, err := json.Marshal(&payload{
		Channel:  s.channel,
		Username: s.username,
		IconURL:  s.iconURL,
		Attachments: []attachment{
			{
				Color: color,
				Text:  fmt.Sprintf(msg, v...),
			},
		},
	})

	if err != nil {
		return err
	}

	s.infof("payload: %s", b)
	r, err := http.Post(s.webhookURL, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	s.infof("response: %s", r.Status)

	if r.StatusCode >= 400 {
		return &ResponseError{r}
	}
	return nil
}

// infof prints a debug message.
func (s *Slack) infof(format string, v ...interface{}) {
	if s.logger != nil {
		s.logger.Printf(format, v...)
	}
}

// ResponseError returned when response code is more than 400.
type ResponseError struct {
	r *http.Response
}

// Error is a string representation.
func (r *ResponseError) Error() string {
	return fmt.Sprintf("slack responded with %d status code", r.r.StatusCode)
}
