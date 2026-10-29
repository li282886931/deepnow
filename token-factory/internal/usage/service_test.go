package usage

import (
	"context"
	"errors"
	"testing"
	"time"
)

type usageRepository struct {
	inserted bool
	events   []UsageEvent
	daily    []DailyUsage
}

func (r *usageRepository) Record(_ context.Context, event UsageEvent) (bool, error) {
	r.events = append(r.events, event)
	return r.inserted, nil
}

func (r *usageRepository) AggregateDaily(_ context.Context, daily DailyUsage) error {
	r.daily = append(r.daily, daily)
	return nil
}

func TestRecordAggregatesAcceptedUsageByUTCDate(t *testing.T) {
	repo := &usageRepository{inserted: true}
	service := NewService(repo)
	event := UsageEvent{
		EventID: "evt_1", RequestID: "req_1", TokenID: "tok_1", Endpoint: "/v1/chat",
		StatusCode: 200, Allowed: true, Reason: "allowed", Latency: 25 * time.Millisecond,
		TokenUnits: 37, OccurredAt: time.Date(2026, 10, 29, 23, 59, 59, 0, time.FixedZone("local", 8*60*60)),
	}
	if err := service.Record(context.Background(), event); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if len(repo.events) != 1 || len(repo.daily) != 1 {
		t.Fatalf("repository writes = events %d daily %d, want one each", len(repo.events), len(repo.daily))
	}
	got := repo.daily[0]
	if !got.Day.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) ||
		got.AcceptedCalls != 1 || got.DeniedCalls != 0 || got.TokenUnits != 37 {
		t.Fatalf("daily usage = %+v, want UTC accepted aggregate", got)
	}
}

func TestRecordAggregatesDeniedUsageWithoutTokenUnits(t *testing.T) {
	repo := &usageRepository{inserted: true}
	service := NewService(repo)
	err := service.Record(context.Background(), UsageEvent{
		EventID: "evt_2", TokenID: "tok_1", Allowed: false, StatusCode: 403,
		Reason: "scope_denied", TokenUnits: 99,
		OccurredAt: time.Date(2026, 10, 30, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	got := repo.daily[0]
	if got.AcceptedCalls != 0 || got.DeniedCalls != 1 || got.TokenUnits != 0 {
		t.Fatalf("daily usage = %+v, want denied call without token units", got)
	}
}

func TestRecordDoesNotAggregateDuplicateEvent(t *testing.T) {
	repo := &usageRepository{inserted: false}
	service := NewService(repo)
	if err := service.Record(context.Background(), UsageEvent{EventID: "evt_duplicate"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if len(repo.daily) != 0 {
		t.Fatalf("daily writes = %d, want no write for duplicate event", len(repo.daily))
	}
}

func TestRecordReturnsRepositoryFailure(t *testing.T) {
	repo := &failingUsageRepository{err: errors.New("database unavailable")}
	if err := NewService(repo).Record(context.Background(), UsageEvent{EventID: "evt_3"}); err == nil {
		t.Fatal("Record() error = nil, want repository failure")
	}
}

type failingUsageRepository struct{ err error }

func (r *failingUsageRepository) Record(context.Context, UsageEvent) (bool, error) {
	return false, r.err
}

func (r *failingUsageRepository) AggregateDaily(context.Context, DailyUsage) error {
	return nil
}
