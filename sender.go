package slack_threads

import (
	"fmt"
	"github.com/slack-go/slack"
	"log/slog"
	"sync"
	"time"
)

type Sender struct {
	*slack.Client
	*slack.PostMessageParameters
	FallbackChannel string
	Timeout         time.Duration
}

func InChannel(channel string) *Sender {
	c := &Sender{
		Client: defaultSender.Client,
		PostMessageParameters: &slack.PostMessageParameters{
			Channel: channel,
		},
		FallbackChannel: defaultSender.FallbackChannel,
	}
	return c
}

func (s *Sender) Customize(username string, iconEmoji string) *Sender {
	s.Username = username
	s.IconEmoji = iconEmoji
	return s
}

func (s *Sender) postMessage(threadTs string, text string) (channel string, ts string, err error) {
	if s.PostMessageParameters.Channel == "" {
		err = fmt.Errorf("destination channel not set")
		return
	}

	// actually send the request
	channel, ts, err = s.Client.PostMessage(
		s.PostMessageParameters.Channel,
		slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTs),
	)

	// TODO: fallback handling

	return
}

func (s *Sender) PostMessage(text string) (err error) {
	_, _, err = s.postMessage("", text)
	return
}

func (s *Sender) PostMessageThread(texts []string) (err error) {
	threadTs := ""
	for _, text := range texts {
		_, threadTs, err = s.postMessage(threadTs, text)
		if err != nil {
			return
		}
	}
	return
}

type DynamicMessage struct {
	// doing something with arrays or channels is more powerful, but we usually don't need the flexibility
	Placeholder func() string
	Eventual    func() (string, []slack.Block, error)
	OnFailure   func(error) string
}

type DynamicSummary struct {
	// doing something with arrays or channels is more powerful, but we usually don't need the flexibility
	Placeholder func() string
	Eventual    func(total int, failures int) (string, []slack.Block, error)
	OnFailure   func(error) string
}

func (s *Sender) postMessageDynamic(threadTs string, msg DynamicMessage, wg *sync.WaitGroup) (channel string, msgTs string, err error) {
	var text string
	channel = s.Channel

	if msg.Placeholder != nil {
		text = msg.Placeholder()

		// try to post the placeholder message
		channel, msgTs, err = s.Client.PostMessage(
			channel,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionTS(threadTs),
		)

		if err != nil {
			slog.Warn(fmt.Sprintf("failed posting Placeholder: %v", err))
		}
	}

	if msg.Eventual == nil {
		wg.Done()
		return
	}

	go func() {
		text, blocks, err := msg.Eventual()
		if err != nil {
			if msg.OnFailure != nil {
				// let's try getting a customized failure message
				text = msg.OnFailure(err)
				blocks = []slack.Block{}
			} else {
				text = "Something went wrong generating this message."
				blocks = []slack.Block{}
			}
		}

		if msgTs == "" {
			_, msgTs, err = s.Client.PostMessage(
				channel,
				slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
				slack.MsgOptionText(text, false),
				slack.MsgOptionBlocks(blocks...),
				slack.MsgOptionTS(CoalesceStr(threadTs, msgTs)),
			)
			if err != nil {
				//return "", "", fmt.Errorf("failed posting to Slack: %w", err)
			}
		} else {
			_, _, _, err = s.Client.UpdateMessage(
				// UpdateMessage REQUIRES a channel ID
				// channel name is not good enough here.
				channel, msgTs,
				slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
				slack.MsgOptionText(text, false),
				slack.MsgOptionBlocks(blocks...),
				slack.MsgOptionTS(CoalesceStr(threadTs, msgTs)),
			)
			if err != nil {
				//return "", "", fmt.Errorf("failed updating Slack message: %w", err)
			}
		}

		wg.Done()
	}()

	return
}

func (s *Sender) PostMessageDynamic(msg DynamicMessage) (err error) {
	var wg *sync.WaitGroup
	_, _, err = s.postMessageDynamic("", msg, wg)
	return
}

// messages with placeholders will always be posted in order
// messages without placeholders will be posted in the order they complete
func (s *Sender) PostThreadDynamic(rootMsg DynamicSummary, msgs ...DynamicMessage) (finalChannel string, threadRootTs string, finalErr error) {
	channel := s.Channel

	// prepare channels for receiving each generated message
	placeholdersTs := make([]string, len(msgs))
	texts := make([]string, len(msgs))
	blockGroups := make([][]slack.Block, len(msgs))
	errs := make([]chan error, len(msgs))
	for i := range errs {
		errs[i] = make(chan error)
	}

	// start generating results for all messages in parallel
	remaining := 0
	for i, v := range msgs {
		msg := v
		msgIndex := i // intermediate variable to make goroutine safe
		if msg.Eventual == nil {
			continue
		}
		remaining++
		go func() {
			msgText, msgBlocks, msgErr := msg.Eventual()
			if msgErr == nil {
				texts[msgIndex] = msgText
				blockGroups[msgIndex] = msgBlocks
			} else {
				texts[msgIndex] = msg.OnFailure(msgErr)
			}
			errs[msgIndex] <- msgErr
		}()
	}

	// post the thread placeholder right away
	{
		text := "preparing thread..."
		if rootMsg.Placeholder != nil {
			text = rootMsg.Placeholder()
		}
		var err error
		finalChannel, threadRootTs, err = s.Client.PostMessage(
			channel,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionTS(threadRootTs),
		)
		if err != nil {
			finalErr = fmt.Errorf("failed to start thread: %w", err)
			return
		}
	}

	// post all the dynamic placeholders that are available
	for i, msg := range msgs {
		if msg.Placeholder == nil {
			continue
		}

		text := msg.Placeholder()

		var ts string
		var err error
		finalChannel, ts, err = s.Client.PostMessage(
			channel,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionTS(threadRootTs),
		)
		if err == nil {
			placeholdersTs[i] = ts
			//if threadRootTs == "" {
			//	threadRootTs = ts
			//}
		} else {
			slog.Warn("failed to post placeholder")
		}
	}

	// loop until all the messages have been finalized
	failures := 0
	timeout := max(time.Minute, s.Timeout) // short deadlines are unreasonable
	deadline := time.Now().Add(timeout)
	for remaining > 0 {
		for i, msg := range msgs {
			select {
			case err := <-errs[i]:
				remaining--
				var text string
				var blockGroup []slack.Block
				if err == nil {
					text = texts[i]
					blockGroup = blockGroups[i]
				} else {
					text = msg.OnFailure(err)
					blockGroup = []slack.Block{}
					failures++
				}
				if placeholdersTs[i] == "" {
					finalChannel, _, err = s.Client.PostMessage(
						channel,
						slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
						slack.MsgOptionText(text, false),
						slack.MsgOptionTS(threadRootTs),
					)
					//if threadRootTs == "" && ts != "" {
					//	threadRootTs = ts
					//}
				} else {
					finalChannel, _, _, err = s.Client.UpdateMessage(
						// UpdateMessage REQUIRES a channel ID
						// channel name is not good enough here.
						finalChannel, placeholdersTs[i],
						slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
						slack.MsgOptionText(text, false),
						slack.MsgOptionBlocks(blockGroup...),
						slack.MsgOptionTS(CoalesceStr(threadRootTs, placeholdersTs[i])),
					)
				}
				if err != nil {
					slog.Error("failed posting to slack", "error", err)
					// TODO: what kinds of errors are retryable?
					// this would be a great use of sets
					if err.Error() == "channel_not_found" {
						return
					} else {
						continue
					}
				}
			default:
				continue
			}
		}

		if time.Now().After(deadline) {
			finalErr = fmt.Errorf("failed building thread: deadline exceeded")
			return
		}

		// sleep a bit so this loop isn't too hot
		time.Sleep(50 * time.Millisecond)
	}

	if rootMsg.Eventual != nil {
		text, blockGroup, err := rootMsg.Eventual(len(msgs), failures)
		if err != nil {
			text = rootMsg.OnFailure(err)
			blockGroup = nil
			return
		}
		_, threadRootTs, _, err = s.Client.UpdateMessage(
			// UpdateMessage REQUIRES a channel ID
			// channel name is not good enough here.
			finalChannel, threadRootTs,
			slack.MsgOptionPostMessageParameters(*s.PostMessageParameters),
			slack.MsgOptionText(text, false),
			slack.MsgOptionBlocks(blockGroup...),
			slack.MsgOptionTS(threadRootTs),
		)
		if err != nil {
			finalErr = fmt.Errorf("failed updating thread summary: %w", err)
			return
		}

	}

	return

	//var wg sync.WaitGroup
	//var ts string
	//wg.Add(1)
	//_, ts, err = s.postMessageDynamic(ts, msgs[0], &wg)
	//
	//// TODO: run all the slow jobs in parallel goroutines
	//for i, msg := range msgs[1:] {
	//	wg.Add(1)
	//	_, _, err = s.postMessageDynamic(ts, msg, &wg)
	//	if err != nil {
	//		return fmt.Errorf("failed posting dynamic message %d/%d in thread: %w", i+1, len(msgs), err)
	//	}
	//}
	//wg.Wait()
	//return
}

// ordering is optional
// fail fast is optional
// dynamic? first, replacement, error message
//func (s *Sender) PostThreadFn(channel string, text []string, mReqFns ...func() slack.PostMessageParameters) (chan Message, chan error) {
//	responses := make(chan Message, len(mReqFns))
//	errors := make(chan error, len(mReqFns))
//	go func() {
//		for i, mReq := range mReqFns {
//			msg, err := PostMessage(channel, text[i], mReq())
//			if err != nil {
//				errors <- err
//			} else {
//				responses <- msg
//			}
//		}
//	}()
//	return responses, errors
//}
