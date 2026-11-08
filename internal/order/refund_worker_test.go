package order

import (
	"testing"
	"time"
)

func TestRetryBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{4, 40 * time.Second},
		{5, 80 * time.Second},
		{7, 320 * time.Second},
		{8, 10 * time.Minute},
		{20, 10 * time.Minute},
	}
	for _, tc := range cases {
		if got := retryBackoff(tc.attempt); got != tc.want {
			t.Fatalf("retryBackoff(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}
