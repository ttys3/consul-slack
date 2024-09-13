package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
)

// New creates new slack client.
func New(url string) (*Discord, error) {
	s := &Discord{
		webhookURL: url,
		logger:     slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo})),
	}
	return s, nil
}

// Discord is a slack client.
type Discord struct {
	webhookURL string
	iconURL    string
	logger     *slog.Logger
}

// Danger is equivalent of Send("danger", ...)
func (s *Discord) Danger(msg string, v ...interface{}) error {
	return s.Send(0xE01563, msg, v...)
}

// Good is equivalent of Send("good", ...)
func (s *Discord) Good(msg string, v ...interface{}) error {
	return s.Send(0x3eb991, msg, v...)
}

// Warning is equivalent of Send("warning", ...)
func (s *Discord) Warning(msg string, v ...interface{}) error {
	return s.Send(0xE9A820, msg, v...)
}

// Message is equivalent of Send("", ...), no color.
func (s *Discord) Message(msg string, v ...interface{}) error {
	return s.Send(0, msg, v...)
}

// Send sends message to the webhook url.
func (s *Discord) Send(color int, msg string, v ...interface{}) error {
	b, err := json.Marshal(&discordgo.MessageSend{
		Content: fmt.Sprintf("consul check event %s", time.Now().Local().String()),
		Embeds: []*discordgo.MessageEmbed{
			{
				Color: color,
				Title: fmt.Sprintf(msg, v...),
			},
		},
	})
	if err != nil {
		return err
	}

	s.logger.Info("send payload", "payload", b)
	r, err := http.Post(s.webhookURL, "application/json", bytes.NewReader(b))
	if err != nil {
		s.logger.Error("failed to send discord message", "err", err)
		return err
	}
	s.logger.Info("response", "status_code", r.Status)

	if r.StatusCode >= 400 {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.logger.Error("failed to read response message", "err", err)
			return fmt.Errorf("err: %w", err)
		}

		s.logger.Error("failed to send discord message", "body", string(body))
		return &ResponseError{r}
	}
	return nil
}

// ResponseError returned when response code is more than 400.
type ResponseError struct {
	r *http.Response
}

// Error is a string representation.
func (r *ResponseError) Error() string {
	return fmt.Sprintf("slack responded with %d status code", r.r.StatusCode)
}
