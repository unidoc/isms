package db

import (
	"context"
	"testing"
)

// TestChangelogAPIKeyID covers changelogAPIKeyID's precedence rules for
// attributing a changelog write to an API key (#335): an explicit value on
// the entry always wins, otherwise the key carried on ctx (set by
// WithAPIKeyID) is used, and a zero id on ctx is never recorded.
func TestChangelogAPIKeyID(t *testing.T) {
	explicit3 := 3

	type otherKey struct{}

	tests := []struct {
		name     string
		ctx      context.Context
		explicit *int
		want     *int
	}{
		{
			name:     "plain context, no explicit value",
			ctx:      context.Background(),
			explicit: nil,
			want:     nil,
		},
		{
			name:     "key on context, no explicit value",
			ctx:      WithAPIKeyID(context.Background(), 7),
			explicit: nil,
			want:     intPtr(7),
		},
		{
			name:     "key on context, explicit value wins",
			ctx:      WithAPIKeyID(context.Background(), 7),
			explicit: &explicit3,
			want:     intPtr(3),
		},
		{
			name:     "zero id on context is never recorded",
			ctx:      WithAPIKeyID(context.Background(), 0),
			explicit: nil,
			want:     nil,
		},
		{
			name:     "key survives a child context.WithValue",
			ctx:      context.WithValue(WithAPIKeyID(context.Background(), 7), otherKey{}, "x"),
			explicit: nil,
			want:     intPtr(7),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changelogAPIKeyID(tt.ctx, tt.explicit)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("changelogAPIKeyID() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("changelogAPIKeyID() = nil, want %d", *tt.want)
			}
			if *got != *tt.want {
				t.Fatalf("changelogAPIKeyID() = %d, want %d", *got, *tt.want)
			}
		})
	}
}
