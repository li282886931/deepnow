package usage

import (
	"context"
	"time"
)

type UsageEvent struct {
	EventID    string
	RequestID  string
	Endpoint   string
	TokenID    string
	StatusCode int
	Allowed    bool
	Reason     string
	Latency    time.Duration
	TokenUnits int64
	OccurredAt time.Time
}

type DailyUsage struct {
	TokenID       string
	Day           time.Time
	AcceptedCalls int64
	DeniedCalls   int64
	TokenUnits    int64
}

type Repository interface {
	Record(context.Context, UsageEvent) (inserted bool, err error)
	AggregateDaily(context.Context, DailyUsage) error
}

type Service struct {
	repo Repository
	Now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		Now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Record(ctx context.Context, event UsageEvent) error {
	inserted, err := s.repo.Record(ctx, event)
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}
	at := event.OccurredAt
	if at.IsZero() {
		at = s.Now()
	}
	at = at.UTC()
	daily := DailyUsage{
		TokenID: event.TokenID,
		Day:     time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
	}
	if event.Allowed {
		daily.AcceptedCalls = 1
		if event.TokenUnits > 0 {
			daily.TokenUnits = event.TokenUnits
		}
	} else {
		daily.DeniedCalls = 1
	}
	return s.repo.AggregateDaily(ctx, daily)
}
