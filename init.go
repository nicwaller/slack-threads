package slack_threads

import (
	"github.com/slack-go/slack"
	"log/slog"
	"os"
	"time"
)

var defaultSender *Sender

func init() {
	// NOTE: good place to choose the best Slack workspace for messages from this app
	token := CoalesceStr(
		os.Getenv("SLACK_API_TOKEN"),
		os.Getenv("SLACK_BOT_TOKEN"),
	)
	if token == "" {
		slog.Error("no Slack API token")
	}
	defaultSender = &Sender{
		apiClient: slack.New(token),
		PostMessageParameters: &slack.PostMessageParameters{
			Markdown: true,
			// TODO: pre-populate metadata with useful information like hostname, IP address
		},
		FallbackChannel: "",
		Timeout:         time.Minute,
	}
}
