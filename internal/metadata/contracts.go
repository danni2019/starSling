package metadata

import (
	"fmt"
	"strings"
)

func LoadContractInstrumentIDs() ([]string, error) {
	cached, err := Load("contract")
	if err != nil {
		return nil, err
	}
	rows, err := parseContractRows(cached.Data)
	if err != nil {
		return nil, fmt.Errorf("parse contract metadata: %w", err)
	}
	seen := make(map[string]struct{}, len(rows))
	out := make([]string, 0, len(rows))
	for _, item := range rows {
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
