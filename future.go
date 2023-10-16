package slack_threads

import (
	"fmt"
	"github.com/slack-go/slack"
	"log/slog"
	"sync"
)

type MessageFuture struct {
	// Placeholder text is posted as soon as possible to preserve message ordering
	// Use this when you want to send feedback as soon as possible
	// or when sending multiple messages and ordering is important
	Placeholder func() string

	// Eventual allows for text and Blocks after a period of processing
	Eventual func() (string, []slack.Block, error)

	// OnFailure provides text in case Eventual returns an error
	OnFailure func(error) string
}

type SummaryGenerator func(total int, succeeded int) string

func DefaultSummarizer(total int, succeeded int) (text string) {
	text = ":thread: Preparing responses... "
	if total == 0 {
		return
	}
	failures := total - succeeded
	if failures == 0 {
		text += fmt.Sprintf("%d/%d done! :white_check_mark:", succeeded, total)
	} else {
		text += fmt.Sprintf("%d succeeded but %d failed. :x:", succeeded, failures)
	}
	return

}

func (s *Sender) PostThreadFutureWithSummarizer(summary SummaryGenerator, msgs ...*MessageFuture) (thread *Thread, err error) {
	if summary == nil {
		summary = DefaultSummarizer
	}

	{
		var channel string
		var ts string
		channel, ts, err = s.apiClient.PostMessage(s.Channel,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(summary(0, 0), false),
		)
		if err != nil {
			return
		}
		thread = &Thread{
			Channel: &Channel{
				Sender:  s,
				channel: channel,
			},
			Timestamp: ts,
		}
	}

	var placeholders sync.WaitGroup
	var futures sync.WaitGroup
	errorsFromFuture := make(chan error, len(msgs))
	for _, v := range msgs {
		msg := v
		placeholders.Add(1)
		futures.Add(1)
		go func() {
			var resolveErr error
			_, _, resolveErr, _ = s.postMessageFuture(thread.Timestamp, msg, &placeholders)
			errorsFromFuture <- resolveErr
			futures.Done()
		}()
		// Wait for Slack API to acknowledge the placeholder before continuing
		// so that placeholders appear in the same order they're given here.
		placeholders.Wait()
	}
	// Wait for all messages to be fully resolved and posted.
	futures.Wait()
	close(errorsFromFuture)

	total := len(msgs)
	succeeded := 0
	for futureErr := range errorsFromFuture {
		if futureErr == nil {
			succeeded++
		}
	}

	_, _, _, err = s.apiClient.UpdateMessage(thread.channel, thread.Timestamp,
		slack.MsgOptionText(summary(total, succeeded), false),
	)

	return
}

func (s *Sender) PostThreadFuture(rootMsg *MessageFuture, msgs ...*MessageFuture) (thread *Thread, err error) {
	thread, err = s.PostMessageFuture("", rootMsg)
	if err != nil {
		return
	}

	var placeholders sync.WaitGroup
	var futures sync.WaitGroup
	for _, v := range msgs {
		msg := v
		placeholders.Add(1)
		futures.Add(1)
		go func() {
			_, _, _, _ = s.postMessageFuture(thread.Timestamp, msg, &placeholders)
			futures.Done()
		}()
		// Wait for Slack API to acknowledge the placeholder before continuing
		// so that placeholders appear in the same order they're given here.
		placeholders.Wait()
	}
	// Wait for all messages to be fully resolved and posted.
	futures.Wait()

	return
}

func (s *Sender) PostMessageFuture(threadTs string, msg *MessageFuture) (*Thread, error) {
	channel, ts, _, err := s.postMessageFuture(threadTs, msg, nil)
	if err != nil {
		return &Thread{}, err
	}
	t := &Thread{
		Channel: &Channel{
			Sender:  s,
			channel: channel,
		},
		Timestamp: ts,
	}
	return t, nil
}

func (s *Sender) postMessageFuture(
	threadTs string, msg *MessageFuture, waitPlaceholder *sync.WaitGroup) (
	channel string, timestamp string, resolveErr error, err error) {

	var text string
	channel = s.Channel

	if msg.Placeholder != nil {
		text = msg.Placeholder()

		// try to post the placeholder message
		channel, timestamp, err = s.apiClient.PostMessage(
			channel,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionTS(threadTs),
		)

		if err != nil {
			slog.Warn(fmt.Sprintf("failed posting Placeholder: %v", err))
		}
	}

	if waitPlaceholder != nil {
		waitPlaceholder.Done()
	}

	if msg.Eventual == nil {
		return
	}

	text, blocks, resolveErr := msg.Eventual()
	if resolveErr != nil {
		if msg.OnFailure != nil {
			// let's try getting a customized failure message
			text = msg.OnFailure(resolveErr)
			blocks = []slack.Block{}
		} else {
			text = "Something went wrong generating this message."
			blocks = []slack.Block{}
		}
	}

	if timestamp == "" {
		_, timestamp, err = s.apiClient.PostMessage(
			channel,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionTS(CoalesceStr(threadTs, timestamp)),
		)
	} else {
		_, _, _, err = s.apiClient.UpdateMessage(
			// UpdateMessage REQUIRES a channel ID
			// channel name is not good enough here.
			channel, timestamp,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionTS(CoalesceStr(threadTs, timestamp)),
		)
	}

	return
}
