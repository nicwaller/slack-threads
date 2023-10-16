package slack_threads

import (
	"github.com/slack-go/slack"
	"time"
)

type Sender struct {
	apiClient *slack.Client
	*slack.PostMessageParameters
	FallbackChannel string
	Timeout         time.Duration
}

func (s *Sender) Customize(username string, iconEmoji string) {
	s.Username = username
	s.IconEmoji = iconEmoji
}

func Customize(username string, iconEmoji string) {
	defaultSender.Username = username
	defaultSender.IconEmoji = iconEmoji
}

type Channel struct {
	*Sender
	channel string
}

func InChannel(channel string) *Channel {
	c := &Channel{
		channel: channel,
	}
	c.Sender = &Sender{
		apiClient:             defaultSender.apiClient,
		PostMessageParameters: defaultSender.PostMessageParameters, // FIXME: wanted deep copy but got reference
		FallbackChannel:       defaultSender.FallbackChannel,
	}
	c.Sender.PostMessageParameters.Channel = channel
	return c
}

func (t *Channel) Post(text string) (*Thread, error) {
	channel, ts, err := t.Sender.apiClient.PostMessage(t.channel,
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		return nil, err
	}
	return &Thread{
		Channel: &Channel{
			Sender:  t.Sender,
			channel: channel,
		},
		Timestamp: ts,
	}, nil
}

type Thread struct {
	*Channel
	Timestamp string
}

func InThread(channel string, ts string) *Thread {
	t := &Thread{
		Timestamp: ts,
	}
	t.channel = channel
	t.Sender = &Sender{
		apiClient: defaultSender.apiClient,
		PostMessageParameters: &slack.PostMessageParameters{
			Channel: channel,
		},
		FallbackChannel: defaultSender.FallbackChannel,
	}
	return t
}

func (t *Thread) Post(text string) (*Thread, error) {
	channel, ts, err := t.Sender.apiClient.PostMessage(t.channel,
		slack.MsgOptionTS(t.Timestamp),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		return nil, err
	}
	return &Thread{
		Channel: &Channel{
			Sender:  t.Sender,
			channel: channel,
		},
		Timestamp: ts,
	}, nil
}
