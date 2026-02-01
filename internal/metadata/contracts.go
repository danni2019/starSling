package metadata

import (
	"encoding/json"
	"fmt"
	"strings"
)

type contractPayload struct {
	Data []struct {
		InstrumentID string `json:"InstrumentID"`
	} `json:"data"`
}

func LoadContractInstrumentIDs() ([]string, error) {
	cached, err := Load("contract")
	if err != nil {
		return nil, err
	}
	var payload contractPayload
	if err := json.Unmarshal(cached.Data, &payload); err != nil {
		return nil, fmt.Errorf("parse contract metadata: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("contract metadata empty")
	}
	seen := make(map[string]struct{}, len(payload.Data))
	out := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.InstrumentID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("contract metadata has no instrument ids")
	}
	return out, nil
}
