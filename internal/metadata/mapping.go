package metadata

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

type ContractMapping struct {
	Contract     string
	Symbol       string
	RawSymbol    string
	Underlying   string
	ProductClass string
	OptionType   string
	OptionCP     string
}

type ContractMappings struct {
	byContract                   map[string]ContractMapping
	optionRootToUnderlyingSymbol map[string]string
}

var (
	contractMappingsMu     sync.RWMutex
	contractMappings       *ContractMappings
	leadingContractPattern = regexp.MustCompile(`^[A-Za-z]+\d+`)
)

type contractMetadataEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type contractRow struct {
	InstrumentID      string `json:"InstrumentID"`
	ProductID         string `json:"ProductID"`
	UnderlyingInstrID string `json:"UnderlyingInstrID"`
	ProductClass      string `json:"ProductClass"`
	OptionsType       string `json:"OptionsType"`
}

func LoadContractMappings() (*ContractMappings, error) {
	cached, err := Load("contract")
	if err != nil {
		return nil, err
	}
	rows, err := parseContractRows(cached.Data)
	if err != nil {
		return nil, err
	}
	return newContractMappings(rows), nil
}

func ContractMappingsCache() *ContractMappings {
	contractMappingsMu.RLock()
	defer contractMappingsMu.RUnlock()
	return contractMappings
}

func ReloadContractMappings() (*ContractMappings, error) {
	mappings, err := LoadContractMappings()
	if err != nil {
		return nil, err
	}
	contractMappingsMu.Lock()
	contractMappings = mappings
	contractMappingsMu.Unlock()
	return mappings, nil
}

func newContractMappings(rows []contractRow) *ContractMappings {
	out := &ContractMappings{
		byContract:                   make(map[string]ContractMapping, len(rows)),
		optionRootToUnderlyingSymbol: make(map[string]string),
	}
	for _, row := range rows {
		contract := strings.TrimSpace(row.InstrumentID)
		if contract == "" {
			continue
		}
		rawSymbol := strings.TrimSpace(row.ProductID)
		optionCP := optionTypeToCP(row.OptionsType)
		out.byContract[normalizeContractKey(contract)] = ContractMapping{
			Contract:     contract,
			Symbol:       "",
			RawSymbol:    rawSymbol,
			Underlying:   strings.TrimSpace(row.UnderlyingInstrID),
			ProductClass: strings.TrimSpace(row.ProductClass),
			OptionType:   strings.TrimSpace(row.OptionsType),
			OptionCP:     optionCP,
		}
	}
	for key, mapping := range out.byContract {
		if mapping.ProductClass == "2" {
			continue
		}
		symbol := normalizeProductSymbol(mapping.RawSymbol)
		if symbol == "" {
			symbol = contractRoot(mapping.Contract)
		}
		mapping.Symbol = symbol
		out.byContract[key] = mapping
	}
	for key, mapping := range out.byContract {
		if mapping.ProductClass != "2" {
			continue
		}
		symbol := ""
		if under, ok := out.byContract[normalizeContractKey(mapping.Underlying)]; ok {
			symbol = strings.TrimSpace(under.Symbol)
		}
		if symbol == "" {
			symbol = normalizeOptionProductSymbol(mapping.RawSymbol, mapping.OptionCP)
		}
		if symbol == "" {
			symbol = contractRoot(mapping.Contract)
		}
		mapping.Symbol = symbol
		out.byContract[key] = mapping

		root := strings.ToLower(contractRoot(mapping.Contract))
		if root == "" || symbol == "" {
			continue
		}
		if _, exists := out.optionRootToUnderlyingSymbol[root]; !exists {
			out.optionRootToUnderlyingSymbol[root] = symbol
		}
	}
	return out
}

func parseContractRows(raw json.RawMessage) ([]contractRow, error) {
	payload := json.RawMessage(strings.TrimSpace(string(raw)))
	if len(payload) == 0 || string(payload) == "null" {
		return nil, fmt.Errorf("contract metadata empty")
	}
	switch payload[0] {
	case '[':
		var rows []contractRow
		if err := json.Unmarshal(payload, &rows); err != nil {
			return nil, fmt.Errorf("parse contract metadata array: %w", err)
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("contract metadata empty")
		}
		return rows, nil
	case '{':
		var envelope contractMetadataEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, fmt.Errorf("parse contract metadata envelope: %w", err)
		}
		inner := json.RawMessage(strings.TrimSpace(string(envelope.Data)))
		if len(inner) == 0 || string(inner) == "null" {
			return nil, fmt.Errorf("contract metadata has no data field")
		}
		var rows []contractRow
		if err := json.Unmarshal(inner, &rows); err == nil {
			if len(rows) == 0 {
				return nil, fmt.Errorf("contract metadata empty")
			}
			return rows, nil
		}
		var wrapped struct {
			Data []contractRow `json:"data"`
		}
		if err := json.Unmarshal(inner, &wrapped); err != nil {
			return nil, fmt.Errorf("parse contract metadata data: %w", err)
		}
		if len(wrapped.Data) == 0 {
			return nil, fmt.Errorf("contract metadata empty")
		}
		return wrapped.Data, nil
	default:
		return nil, fmt.Errorf("unsupported contract metadata payload")
	}
}

func normalizeContractKey(contract string) string {
	return strings.ToLower(strings.TrimSpace(contract))
}

func optionTypeToCP(optionType string) string {
	token := strings.ToLower(strings.TrimSpace(optionType))
	switch token {
	case "1", "c", "call", "认购":
		return "c"
	case "2", "p", "put", "认沽":
		return "p"
	default:
		return ""
	}
}

func normalizeProductSymbol(symbol string) string {
	symbol = strings.TrimSpace(symbol)
	if idx := strings.Index(symbol, "_"); idx >= 0 {
		symbol = symbol[:idx]
	}
	return strings.TrimSpace(symbol)
}

func normalizeOptionProductSymbol(symbol string, optionCP string) string {
	base := normalizeProductSymbol(symbol)
	if len(base) <= 1 {
		return base
	}
	switch strings.ToLower(strings.TrimSpace(optionCP)) {
	case "c":
		if strings.EqualFold(base[len(base)-1:], "c") {
			base = base[:len(base)-1]
		}
	case "p":
		if strings.EqualFold(base[len(base)-1:], "p") {
			base = base[:len(base)-1]
		}
	}
	return strings.TrimSpace(base)
}

func contractRoot(contract string) string {
	contract = strings.TrimSpace(contract)
	if contract == "" {
		return ""
	}
	idx := 0
	for idx < len(contract) {
		ch := contract[idx]
		isUpper := ch >= 'A' && ch <= 'Z'
		isLower := ch >= 'a' && ch <= 'z'
		if !isUpper && !isLower {
			break
		}
		idx++
	}
	if idx == 0 {
		return ""
	}
	return contract[:idx]
}

func optionContractCPIndex(contract string) int {
	upper := strings.ToUpper(strings.TrimSpace(contract))
	if len(upper) < 3 {
		return -1
	}
	if idx := strings.LastIndex(upper, "-C-"); idx > 0 {
		return idx + 1
	}
	if idx := strings.LastIndex(upper, "-P-"); idx > 0 {
		return idx + 1
	}
	for i := len(upper) - 2; i >= 1; i-- {
		ch := upper[i]
		if ch != 'C' && ch != 'P' {
			continue
		}
		suffix := upper[i+1:]
		if suffix == "" {
			continue
		}
		allDigits := true
		for _, c := range suffix {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			continue
		}
		prefix := upper[:i]
		if !strings.ContainsAny(prefix, "0123456789") {
			continue
		}
		return i
	}
	return -1
}

func inferOptionTypeCPFromContract(contract string) string {
	trimmed := strings.TrimSpace(contract)
	idx := optionContractCPIndex(trimmed)
	if idx < 0 {
		return ""
	}
	if strings.ToUpper(trimmed)[idx] == 'C' {
		return "c"
	}
	return "p"
}

func inferOptionUnderlyingFromContract(contract string) string {
	trimmed := strings.TrimSpace(contract)
	idx := optionContractCPIndex(trimmed)
	if idx <= 0 {
		return ""
	}
	underlying := strings.TrimRight(strings.TrimSpace(trimmed[:idx]), "-_")
	if token := leadingContractPattern.FindString(underlying); token != "" {
		return token
	}
	return underlying
}

func replaceContractRoot(contract, symbol string) (string, bool) {
	contract = strings.TrimSpace(contract)
	symbol = strings.TrimSpace(symbol)
	if contract == "" || symbol == "" {
		return "", false
	}
	root := contractRoot(contract)
	if root == "" {
		return "", false
	}
	return symbol + contract[len(root):], true
}

func (m *ContractMappings) Lookup(contract string) (ContractMapping, bool) {
	if m == nil {
		return ContractMapping{}, false
	}
	value, ok := m.byContract[normalizeContractKey(contract)]
	if !ok {
		return ContractMapping{}, false
	}
	return value, true
}

func (m *ContractMappings) ResolveContractSymbol(contract string) (string, bool) {
	mapping, ok := m.Lookup(contract)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(mapping.Symbol) == "" {
		return "", false
	}
	return mapping.Symbol, true
}

func (m *ContractMappings) ResolveOptionUnderlying(contract string) (string, bool) {
	mapping, ok := m.Lookup(contract)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(mapping.ProductClass) != "2" {
		return "", false
	}
	if strings.TrimSpace(mapping.Underlying) == "" {
		return "", false
	}
	return mapping.Underlying, true
}

func (m *ContractMappings) ResolveOptionTypeCP(contract string) (string, bool) {
	mapping, ok := m.Lookup(contract)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(mapping.OptionCP) == "" {
		return "", false
	}
	return mapping.OptionCP, true
}

func (m *ContractMappings) InferContractSymbol(contract string) (string, bool) {
	root := contractRoot(contract)
	if root == "" {
		return "", false
	}
	if m != nil {
		if symbol := strings.TrimSpace(m.optionRootToUnderlyingSymbol[strings.ToLower(root)]); symbol != "" {
			return symbol, true
		}
	}
	return root, true
}

func (m *ContractMappings) InferOptionUnderlying(contract string) (string, bool) {
	underlying := inferOptionUnderlyingFromContract(contract)
	if underlying == "" {
		return "", false
	}
	if m != nil {
		root := strings.ToLower(contractRoot(contract))
		if root != "" {
			if symbol := strings.TrimSpace(m.optionRootToUnderlyingSymbol[root]); symbol != "" {
				if replaced, ok := replaceContractRoot(underlying, symbol); ok {
					return replaced, true
				}
			}
		}
	}
	return underlying, true
}

func (m *ContractMappings) InferOptionTypeCP(contract string) (string, bool) {
	cp := inferOptionTypeCPFromContract(contract)
	if cp == "" {
		return "", false
	}
	return cp, true
}
