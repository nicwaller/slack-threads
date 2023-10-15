package main

import (
	"github.com/nicwaller/slack_threads"
)

func main() {

	s := slack_threads.InChannel("test-3").
		Customize("Slack Robot", "robot_face")

	_ = s.PostMessage("Hello, World!")
	_ = s.PostMessageThread([]string{
		":thread: starting a thread",
		"more details in the thread",
		"conclusion",
	})

}
