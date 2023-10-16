# slack-threads

A Go module that helps post threaded messages to Slack.

Especially useful when you want to post a thread of messages that take a while to generate.

## Usage

Choose a channel.

```go
general := slack_threads.InChannel("#general")
```

Post a message.

```go
general.Post("Hello, World!")
```

Use a `MessageFuture` to post a message that automatically updates after a process completes.

```go
fortyTwo := &slack_threads.MessageFuture{
  Placeholder: func() string {
    return "Computing answer..."
  },
  Eventual: func() (string, []slack.Block, error) {
    time.Sleep(30 * time.Second)
    return "42", nil, nil
  },
}

general.PostMessageFuture("", fortyTwo)
```

Create a thread with several message futures and automatic result summarization.

```go
thread, _ = general.PostThreadFutureWithSummarizer(slack_threads.DefaultSummarizer, fortyTwo, fortyTwo)
```

Post a follow-up message to notify somebody after all the futures have resolved and been posted.

```go
thread.Post("Finished deriving the answer to everything. <@U02BJDJN676>")
```
