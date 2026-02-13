package metadata

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestParseContractRowsSupportsWrappedPayload(t *testing.T) {
	raw := json.RawMessage(`{
		"rsp_code": 0,
		"rsp_message": "ok",
		"data": [
			{
				"InstrumentID": "MO2604-C-8000",
				"ProductID": "MO",
				"UnderlyingInstrID": "IM2604",
				"ProductClass": "2",
				"OptionsType": "1"
			}
		]
	}`)
	rows, err := parseContractRows(raw)
	if err != nil {
		t.Fatalf("parse wrapped payload failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].InstrumentID != "MO2604-C-8000" {
		t.Fatalf("unexpected contract %q", rows[0].InstrumentID)
	}
}

func TestParseContractRowsSupportsArrayPayload(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"InstrumentID": "cu2604",
			"ProductID": "cu",
			"UnderlyingInstrID": "cu",
			"ProductClass": "1",
			"OptionsType": ""
		}
	]`)
	rows, err := parseContractRows(raw)
	if err != nil {
		t.Fatalf("parse array payload failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].InstrumentID != "cu2604" {
		t.Fatalf("unexpected contract %q", rows[0].InstrumentID)
	}
}

func TestContractMappingsResolveFromMetadata(t *testing.T) {
	mappings := newContractMappings([]contractRow{
		{
			InstrumentID:      "MO2604-C-8000",
			ProductID:         "MO",
			UnderlyingInstrID: "IM2604",
			ProductClass:      "2",
			OptionsType:       "1",
		},
		{
			InstrumentID:      "IM2604",
			ProductID:         "IM",
			UnderlyingInstrID: "IM",
			ProductClass:      "1",
		},
		{
			InstrumentID:      "SR605-C-7000",
			ProductID:         "sr_o",
			UnderlyingInstrID: "SR605",
			ProductClass:      "2",
			OptionsType:       "1",
		},
		{
			InstrumentID:      "SR605",
			ProductID:         "SR",
			UnderlyingInstrID: "SR",
			ProductClass:      "1",
		},
		{
			InstrumentID:      "AP401C3500",
			ProductID:         "APC",
			UnderlyingInstrID: "",
			ProductClass:      "2",
			OptionsType:       "1",
		},
	})

	if underlying, ok := mappings.ResolveOptionUnderlying("mo2604-c-8000"); !ok || underlying != "IM2604" {
		t.Fatalf("ResolveOptionUnderlying() = (%q,%v), want (IM2604,true)", underlying, ok)
	}
	if symbol, ok := mappings.ResolveContractSymbol("MO2604-C-8000"); !ok || symbol != "IM" {
		t.Fatalf("ResolveContractSymbol(MO option) = (%q,%v), want (IM,true)", symbol, ok)
	}
	if symbol, ok := mappings.ResolveContractSymbol("SR605-C-7000"); !ok || symbol != "SR" {
		t.Fatalf("ResolveContractSymbol(SR option) = (%q,%v), want (SR,true)", symbol, ok)
	}
	if symbol, ok := mappings.ResolveContractSymbol("AP401C3500"); !ok || symbol != "AP" {
		t.Fatalf("ResolveContractSymbol(AP option without underlying) = (%q,%v), want (AP,true)", symbol, ok)
	}
	if symbol, ok := mappings.ResolveContractSymbol("im2604"); !ok || symbol != "IM" {
		t.Fatalf("ResolveContractSymbol(future) = (%q,%v), want (IM,true)", symbol, ok)
	}
	if cp, ok := mappings.ResolveOptionTypeCP("MO2604-C-8000"); !ok || cp != "c" {
		t.Fatalf("ResolveOptionTypeCP() = (%q,%v), want (c,true)", cp, ok)
	}
}

func TestContractMappingsInferOptionFormatsAndIrregularMapping(t *testing.T) {
	mappings := newContractMappings([]contractRow{
		{
			InstrumentID:      "IM2604",
			ProductID:         "IM",
			UnderlyingInstrID: "IM",
			ProductClass:      "1",
		},
		{
			InstrumentID:      "MO2604-C-8000",
			ProductID:         "MO",
			UnderlyingInstrID: "IM2604",
			ProductClass:      "2",
			OptionsType:       "1",
		},
		{
			InstrumentID:      "AB2234",
			ProductID:         "AB",
			UnderlyingInstrID: "AB",
			ProductClass:      "1",
		},
		{
			InstrumentID:      "AB2234-C-123",
			ProductID:         "AB",
			UnderlyingInstrID: "AB2234",
			ProductClass:      "2",
			OptionsType:       "1",
		},
		{
			InstrumentID:      "SR605",
			ProductID:         "SR",
			UnderlyingInstrID: "SR",
			ProductClass:      "1",
		},
		{
			InstrumentID:      "SR605-C-7000",
			ProductID:         "sr_o",
			UnderlyingInstrID: "SR605",
			ProductClass:      "2",
			OptionsType:       "1",
		},
	})

	if symbol, ok := mappings.InferContractSymbol("mo2604c8000"); !ok || symbol != "IM" {
		t.Fatalf("InferContractSymbol(mo2604c8000) = (%q,%v), want (IM,true)", symbol, ok)
	}
	if symbol, ok := mappings.InferContractSymbol("sr605c7000"); !ok || symbol != "SR" {
		t.Fatalf("InferContractSymbol(sr605c7000) = (%q,%v), want (SR,true)", symbol, ok)
	}
	if symbol, ok := mappings.InferContractSymbol("ab2234c123"); !ok || symbol != "AB" {
		t.Fatalf("InferContractSymbol(ab2234c123) = (%q,%v), want (AB,true)", symbol, ok)
	}

	underlyingCases := map[string]string{
		"MO2604-C-8000": "IM2604",
		"mo2604c8000":   "IM2604",
		"AB2234C123":    "AB2234",
		"ab2234-C-123":  "AB2234",
		"SR605-C-7000":  "SR605",
		"sr605c7000":    "SR605",
	}
	for contract, want := range underlyingCases {
		got, ok := mappings.InferOptionUnderlying(contract)
		if !ok || got != want {
			t.Fatalf("InferOptionUnderlying(%q) = (%q,%v), want (%q,true)", contract, got, ok, want)
		}
	}

	cpCases := map[string]string{
		"AB2234-C-123": "c",
		"ab2234c123":   "c",
		"AB2234P123":   "p",
		"ab2234-p-123": "p",
	}
	for contract, want := range cpCases {
		got, ok := mappings.InferOptionTypeCP(contract)
		if !ok || got != want {
			t.Fatalf("InferOptionTypeCP(%q) = (%q,%v), want (%q,true)", contract, got, ok, want)
		}
	}
}

func TestContractMappingsConsistencyAgainstLocalMetadata(t *testing.T) {
	cached, err := Load("contract")
	if err != nil {
		t.Skipf("contract metadata cache unavailable: %v", err)
	}
	rows, err := parseContractRows(cached.Data)
	if err != nil {
		t.Fatalf("parse contract metadata failed: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("contract metadata rows empty")
	}

	mappings := newContractMappings(rows)
	symbolMismatches := 0
	underlyingMismatches := 0
	cpMismatches := 0
	variantMismatches := 0
	samples := make([]string, 0, 20)

	for _, row := range rows {
		contract := strings.TrimSpace(row.InstrumentID)
		if contract == "" {
			continue
		}
		metaSymbol, okMetaSymbol := mappings.ResolveContractSymbol(contract)
		inferSymbol, okInferSymbol := mappings.InferContractSymbol(contract)
		if !okMetaSymbol || !okInferSymbol || normalizeCheckToken(metaSymbol) != normalizeCheckToken(inferSymbol) {
			symbolMismatches++
			samples = appendMismatchSample(samples, fmt.Sprintf("%s symbol: meta=(%q,%v) infer=(%q,%v)", contract, metaSymbol, okMetaSymbol, inferSymbol, okInferSymbol))
		}

		if strings.TrimSpace(row.ProductClass) != "2" {
			continue
		}

		metaUnderlying, okMetaUnderlying := mappings.ResolveOptionUnderlying(contract)
		inferUnderlying, okInferUnderlying := mappings.InferOptionUnderlying(contract)
		if !okMetaUnderlying || !okInferUnderlying || normalizeCheckToken(metaUnderlying) != normalizeCheckToken(inferUnderlying) {
			underlyingMismatches++
			samples = appendMismatchSample(samples, fmt.Sprintf("%s underlying: meta=(%q,%v) infer=(%q,%v)", contract, metaUnderlying, okMetaUnderlying, inferUnderlying, okInferUnderlying))
		}

		metaCP, okMetaCP := mappings.ResolveOptionTypeCP(contract)
		inferCP, okInferCP := mappings.InferOptionTypeCP(contract)
		if !okMetaCP || !okInferCP || normalizeCheckToken(metaCP) != normalizeCheckToken(inferCP) {
			cpMismatches++
			samples = appendMismatchSample(samples, fmt.Sprintf("%s cp: meta=(%q,%v) infer=(%q,%v)", contract, metaCP, okMetaCP, inferCP, okInferCP))
		}

		expectedSymbol := inferSymbol
		expectedUnderlying := inferUnderlying
		expectedCP := inferCP
		for _, variant := range buildOptionContractVariants(contract) {
			vSymbol, okVSymbol := mappings.InferContractSymbol(variant)
			vUnderlying, okVUnderlying := mappings.InferOptionUnderlying(variant)
			vCP, okVCP := mappings.InferOptionTypeCP(variant)
			if !okVSymbol || !okVUnderlying || !okVCP ||
				normalizeCheckToken(vSymbol) != normalizeCheckToken(expectedSymbol) ||
				normalizeCheckToken(vUnderlying) != normalizeCheckToken(expectedUnderlying) ||
				normalizeCheckToken(vCP) != normalizeCheckToken(expectedCP) {
				variantMismatches++
				samples = appendMismatchSample(samples, fmt.Sprintf(
					"%s variant %s: symbol=(%q,%v) underlying=(%q,%v) cp=(%q,%v) expect=(%q,%q,%q)",
					contract, variant,
					vSymbol, okVSymbol,
					vUnderlying, okVUnderlying,
					vCP, okVCP,
					expectedSymbol, expectedUnderlying, expectedCP,
				))
			}
		}
	}

	if symbolMismatches > 0 || underlyingMismatches > 0 || cpMismatches > 0 || variantMismatches > 0 {
		t.Fatalf(
			"metadata/infer consistency mismatches: symbol=%d underlying=%d cp=%d variant=%d samples=%v",
			symbolMismatches,
			underlyingMismatches,
			cpMismatches,
			variantMismatches,
			samples,
		)
	}
}

func normalizeCheckToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func appendMismatchSample(samples []string, sample string) []string {
	if len(samples) >= cap(samples) {
		return samples
	}
	return append(samples, sample)
}

func buildOptionContractVariants(contract string) []string {
	contract = strings.TrimSpace(contract)
	if contract == "" {
		return nil
	}
	variants := map[string]struct{}{
		contract:                  {},
		strings.ToUpper(contract): {},
		strings.ToLower(contract): {},
	}
	idx := optionContractCPIndex(contract)
	if idx > 0 {
		cp := contract[idx]
		if cp == 'c' || cp == 'C' || cp == 'p' || cp == 'P' {
			prefix := strings.TrimRight(contract[:idx], "-_")
			suffix := strings.TrimLeft(contract[idx+1:], "-_")
			if prefix != "" && suffix != "" {
				compact := prefix + string(cp) + suffix
				hyphenated := prefix + "-" + string(cp) + "-" + suffix
				variants[compact] = struct{}{}
				variants[strings.ToUpper(compact)] = struct{}{}
				variants[strings.ToLower(compact)] = struct{}{}
				variants[hyphenated] = struct{}{}
				variants[strings.ToUpper(hyphenated)] = struct{}{}
				variants[strings.ToLower(hyphenated)] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(variants))
	for token := range variants {
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}
