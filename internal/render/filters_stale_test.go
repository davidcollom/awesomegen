package render

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestStaleUsesCalendarAnniversaries(t *testing.T) {
	tests := []struct {
		name   string
		pushed time.Time
		now    time.Time
		months int
		want   bool
	}{
		{
			name:   "not stale one day before the anniversary",
			pushed: date(2026, time.January, 31),
			now:    date(2026, time.March, 1),
			months: 1,
			want:   false,
		},
		{
			name:   "stale after the anniversary day",
			pushed: date(2025, time.March, 1),
			now:    date(2026, time.March, 20),
			months: 12,
			want:   true,
		},
		{
			name:   "stale on the day after the anniversary",
			pushed: date(2026, time.January, 31),
			now:    date(2026, time.March, 4),
			months: 1,
			want:   true,
		},
		{
			name:   "not stale before the anniversary day in the anniversary month",
			pushed: date(2024, time.January, 15),
			now:    date(2026, time.January, 10),
			months: 24,
			want:   false,
		},
		{
			name:   "stale after twenty-four months",
			pushed: date(2024, time.January, 1),
			now:    date(2026, time.February, 1),
			months: 24,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, stale(tt.pushed, tt.now, tt.months))
		})
	}
}
