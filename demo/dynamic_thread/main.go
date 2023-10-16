package main

import (
	"fmt"
	"github.com/nicwaller/slack_threads"
	"github.com/slack-go/slack"
	"math/rand"
	"time"
)

func init() {
	slack_threads.Customize("Slack Dog", "dog")
}

func main() {
	test3 := slack_threads.InChannel("test-3")

	dyns := make([]*slack_threads.MessageFuture, 8)
	for i := range dyns {
		dyns[i] = future(i)
	}

	thread := Must(test3.PostThreadFutureWithSummarizer(slack_threads.DefaultSummarizer, dyns...))
	_, _ = thread.Post("<@U02BJDJN676> your report is done")
}

func future(index int) *slack_threads.MessageFuture {
	prefix := fmt.Sprintf("[%d] computing results...", index)
	return &slack_threads.MessageFuture{
		Placeholder: func() string {
			return prefix
		},
		OnFailure: func(err error) string {
			return prefix + fmt.Sprintf("failed: %v", err)
		},
		Eventual: func() (string, []slack.Block, error) {
			seconds := time.Duration(rand.Float64()*15.0) * time.Second
			time.Sleep(seconds)
			if rand.Float64() > 0.8 {
				return "", nil, fmt.Errorf("randomly induced failure")
			}
			a, b, c := genMessage(seconds)
			return a, b, c
		},
	}
}

func genMessage(delay time.Duration) (text string, blocks []slack.Block, err error) {
	blocks = []slack.Block{
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "40 + 2 = 42",
			},
			nil, nil,
		),
		slack.NewContextBlock("",
			slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: fmt.Sprintf("generated in %v", delay),
			},
		),
	}

	return
}

func Must[T any](result T, err error) T {
	if err != nil {
		panic(err)
	}
	return result
}
