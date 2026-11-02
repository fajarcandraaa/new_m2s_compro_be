package retention

import (
	"testing"
	"time"
)

func TestSubtractMonths(t *testing.T) {
	utc := time.UTC
	cases := []struct {
		now  time.Time
		n    int
		want time.Time
	}{
		// 31 bukan tanggal valid di Februari: pakai hari terakhir bulan itu,
		// bukan normalisasi Go yang bergeser ke 3 Maret.
		{time.Date(2026, 3, 31, 3, 0, 0, 0, utc), 1, time.Date(2026, 2, 28, 3, 0, 0, 0, utc)},
		{time.Date(2024, 3, 31, 3, 0, 0, 0, utc), 1, time.Date(2024, 2, 29, 3, 0, 0, 0, utc)},
		{time.Date(2026, 5, 31, 12, 0, 0, 0, utc), 3, time.Date(2026, 2, 28, 12, 0, 0, 0, utc)},
		{time.Date(2026, 11, 2, 3, 0, 0, 0, utc), 24, time.Date(2024, 11, 2, 3, 0, 0, 0, utc)},
		{time.Date(2026, 1, 15, 3, 0, 0, 0, utc), 24, time.Date(2024, 1, 15, 3, 0, 0, 0, utc)},
	}
	for _, c := range cases {
		if got := SubtractMonths(c.now, c.n); !got.Equal(c.want) {
			t.Errorf("SubtractMonths(%s, %d) = %s, want %s", c.now, c.n, got, c.want)
		}
	}
}
