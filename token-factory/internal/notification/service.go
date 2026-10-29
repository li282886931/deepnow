package notification

import "context"

type Message struct {
	To       string
	Subject  string
	Body     string
	Metadata map[string]string
}

type Sender interface {
	Send(context.Context, Message) error
}

type NoopSender struct{}

func (NoopSender) Send(context.Context, Message) error {
	return nil
}
