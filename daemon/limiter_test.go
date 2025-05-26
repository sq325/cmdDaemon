package daemon

import (
	"testing"
	"time"
)

func TestLimiter_next(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		last     time.Time
		interval time.Duration
		location *time.Location
		wantFunc func(time.Time) bool
	}{
		{
			name:     "zero last time returns current time",
			count:    0,
			last:     time.Time{},
			interval: time.Second,
			location: time.UTC,
			wantFunc: func(result time.Time) bool {
				now := time.Now().In(time.UTC)
				return result.Sub(now) < time.Millisecond && result.Sub(now) > -time.Millisecond
			},
		},
		{
			name:     "count 0 adds 1 second",
			count:    0,
			last:     time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			interval: time.Second,
			location: time.UTC,
			wantFunc: func(result time.Time) bool {
				expected := time.Date(2023, 1, 1, 12, 0, 1, 0, time.UTC)
				return result.Equal(expected)
			},
		},
		{
			name:     "count 1 adds 2 seconds",
			count:    1,
			last:     time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			interval: time.Second,
			location: time.UTC,
			wantFunc: func(result time.Time) bool {
				expected := time.Date(2023, 1, 1, 12, 0, 2, 0, time.UTC)
				return result.Equal(expected)
			},
		},
		{
			name:     "count 3 adds 8 seconds",
			count:    3,
			last:     time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			interval: time.Second,
			location: time.UTC,
			wantFunc: func(result time.Time) bool {
				expected := time.Date(2023, 1, 1, 12, 0, 8, 0, time.UTC)
				return result.Equal(expected)
			},
		},
		{
			name:     "different interval",
			count:    2,
			last:     time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			interval: time.Minute,
			location: time.UTC,
			wantFunc: func(result time.Time) bool {
				expected := time.Date(2023, 1, 1, 12, 4, 0, 0, time.UTC)
				return result.Equal(expected)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Limiter{
				count:    tt.count,
				last:     tt.last,
				interval: tt.interval,
				location: tt.location,
			}
			got := l.next()
			if !tt.wantFunc(got) {
				t.Errorf("Limiter.next() = %v, want function validation failed", got)
			}
		})
	}
}
