package router

import (
	"encoding/json"
	"testing"
)

func TestDecodeObjectParams(t *testing.T) {
	t.Run("empty params", func(t *testing.T) {
		var params GetLatestMarketParams
		if err := decodeObjectParams(nil, &params); err != nil {
			t.Fatalf("decode params: %v", err)
		}
		if params.MinSeq != 0 {
			t.Fatalf("expected default MinSeq=0, got %d", params.MinSeq)
		}
	})

	t.Run("valid object", func(t *testing.T) {
		var params GetLatestMarketParams
		err := decodeObjectParams(json.RawMessage(`{"min_seq":42}`), &params)
		if err != nil {
			t.Fatalf("decode params: %v", err)
		}
		if params.MinSeq != 42 {
			t.Fatalf("expected MinSeq=42, got %d", params.MinSeq)
		}
	})

	t.Run("invalid shape", func(t *testing.T) {
		var params GetLatestMarketParams
		if err := decodeObjectParams(json.RawMessage(`"bad"`), &params); err == nil {
			t.Fatalf("expected decode error for malformed params")
		}
	})
}
