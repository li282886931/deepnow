package notification

import (
	"context"
	"testing"
)

func TestNoopSenderDoesNotSendOrFail(t *testing.T) {
	sender := NoopSender{}
	if err := sender.Send(context.Background(), Message{
		Subject: "risk event", Body: "review required",
	}); err != nil {
		t.Fatalf("NoopSender.Send() error = %v", err)
	}
}
