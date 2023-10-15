package main

import (
	"fmt"
	"github.com/nicwaller/slack_threads"
	"github.com/slack-go/slack"
	"math/rand"
	"os"
	"time"
)

func main() {

	s := slack_threads.InChannel("test-3").
		Customize("Slack Robot", "robot_face")

	summary := slack_threads.DynamicSummary{
		Placeholder: func() string {
			return ":thread: preparing reports..."
		},
		Eventual: func(total int, failures int) (string, []slack.Block, error) {
			text := ":thread: preparing reports... "
			if failures == 0 {
				text += "done! :white_check_mark:"
			} else {
				text += fmt.Sprintf("%d/%d failed. :x:", failures, total)
			}
			return text, nil, nil
		},
	}

	dynRandom := slack_threads.DynamicMessage{
		Placeholder: func() string {
			return "computing results..."
		},
		OnFailure: func(err error) string {
			return fmt.Sprintf("computing results... failed: %v", err)
		},
		Eventual: func() (string, []slack.Block, error) {
			seconds := time.Duration(rand.Float64()*15.0) * time.Second
			fmt.Printf("sleeping %v\n", seconds)
			time.Sleep(seconds)
			if rand.Float64() > 0.9 {
				return "", nil, fmt.Errorf("randomly induced failure")
			}
			a, b, c := genMessage(seconds)
			return a, b, c
		},
	}

	dyns := make([]slack_threads.DynamicMessage, 5)
	for i := range dyns {
		dyns[i] = dynRandom
	}

	_, _, err := s.PostThreadDynamic(summary, dyns...)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
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
