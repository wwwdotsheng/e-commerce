package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestRefundTableNames(t *testing.T) {
	cases := []struct {
		name  string
		model any
		want  string
	}{
		{name: "refunds", model: &Refund{}, want: "refunds"},
		{name: "refund_jobs", model: &RefundJob{}, want: "refund_jobs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := schema.Parse(c.model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("schema.Parse: %v", err)
			}
			if s.Table != c.want {
				t.Fatalf("table = %q, want %q", s.Table, c.want)
			}
		})
	}
}
