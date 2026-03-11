package tui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type arbTokenType int

const (
	arbTokenEOF arbTokenType = iota
	arbTokenNumber
	arbTokenIdent
	arbTokenPlus
	arbTokenMinus
	arbTokenMul
	arbTokenDiv
	arbTokenLParen
	arbTokenRParen
)

type arbToken struct {
	Type  arbTokenType
	Text  string
	Value float64
}

type arbNodeKind int

const (
	arbNodeNumber arbNodeKind = iota
	arbNodeContract
	arbNodeUnary
	arbNodeBinary
)

type arbNode struct {
	Kind         arbNodeKind
	Number       float64
	Contract     string
	ContractNorm string
	Op           arbTokenType
	Child        *arbNode
	Left         *arbNode
	Right        *arbNode
}

type compiledArbitrageExpr struct {
	Raw       string
	Root      *arbNode
	Contracts []string
}

type arbEvalResult struct {
	Value    float64
	Known    bool
	Missing  []string
	Err      error
	HasError bool
}

func compileArbitrageExpression(raw string) (*compiledArbitrageExpr, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("formula cannot be empty")
	}
	trimmed = normalizeArbitrageFormulaText(trimmed)
	tokens, err := tokenizeArbitrage(trimmed)
	if err != nil {
		return nil, err
	}
	parser := arbitrageParser{tokens: tokens}
	root, err := parser.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if tok := parser.peek(); tok.Type != arbTokenEOF {
		return nil, fmt.Errorf("unexpected token %q", tok.Text)
	}
	contractsSet := make(map[string]struct{})
	collectArbitrageContracts(root, contractsSet)
	contracts := make([]string, 0, len(contractsSet))
	for contract := range contractsSet {
		contracts = append(contracts, contract)
	}
	sort.Strings(contracts)
	return &compiledArbitrageExpr{
		Raw:       trimmed,
		Root:      root,
		Contracts: contracts,
	}, nil
}

func normalizeArbitrageFormulaText(raw string) string {
	replacer := strings.NewReplacer(
		"（", "(",
		"）", ")",
		"＋", "+",
		"－", "-",
		"×", "*",
		"÷", "/",
	)
	return replacer.Replace(raw)
}

func tokenizeArbitrage(raw string) ([]arbToken, error) {
	tokens := make([]arbToken, 0, len(raw)/2+1)
	for i := 0; i < len(raw); {
		ch := raw[i]
		if isWhitespace(ch) {
			i++
			continue
		}
		switch ch {
		case '+':
			tokens = append(tokens, arbToken{Type: arbTokenPlus, Text: "+"})
			i++
			continue
		case '-':
			tokens = append(tokens, arbToken{Type: arbTokenMinus, Text: "-"})
			i++
			continue
		case '*':
			tokens = append(tokens, arbToken{Type: arbTokenMul, Text: "*"})
			i++
			continue
		case '/':
			tokens = append(tokens, arbToken{Type: arbTokenDiv, Text: "/"})
			i++
			continue
		case '(':
			tokens = append(tokens, arbToken{Type: arbTokenLParen, Text: "("})
			i++
			continue
		case ')':
			tokens = append(tokens, arbToken{Type: arbTokenRParen, Text: ")"})
			i++
			continue
		case '\'':
			end := i + 1
			for end < len(raw) && raw[end] != '\'' {
				end++
			}
			if end >= len(raw) {
				return nil, fmt.Errorf("unterminated quoted contract")
			}
			value := strings.TrimSpace(raw[i+1 : end])
			if value == "" {
				return nil, fmt.Errorf("empty quoted contract")
			}
			tokens = append(tokens, arbToken{Type: arbTokenIdent, Text: value})
			i = end + 1
			continue
		}

		start := i
		for i < len(raw) {
			c := raw[i]
			if isWhitespace(c) || isArbOperator(c) || c == '(' || c == ')' {
				break
			}
			i++
		}
		if start == i {
			return nil, fmt.Errorf("invalid token at position %d", start+1)
		}
		word := strings.TrimSpace(raw[start:i])
		if word == "" {
			return nil, fmt.Errorf("invalid token near position %d", start+1)
		}
		if isNumberLiteral(word) {
			value, err := strconv.ParseFloat(word, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q", word)
			}
			tokens = append(tokens, arbToken{Type: arbTokenNumber, Text: word, Value: value})
		} else {
			tokens = append(tokens, arbToken{Type: arbTokenIdent, Text: word})
		}
	}
	tokens = append(tokens, arbToken{Type: arbTokenEOF, Text: ""})
	return tokens, nil
}

func isArbOperator(ch byte) bool {
	return ch == '+' || ch == '-' || ch == '*' || ch == '/'
}

func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isNumberLiteral(raw string) bool {
	if raw == "" {
		return false
	}
	seenDigit := false
	seenDot := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch >= '0' && ch <= '9' {
			seenDigit = true
			continue
		}
		if ch == '.' && !seenDot {
			seenDot = true
			continue
		}
		return false
	}
	return seenDigit
}

type arbitrageParser struct {
	tokens []arbToken
	pos    int
}

func (p *arbitrageParser) peek() arbToken {
	if p.pos < 0 || p.pos >= len(p.tokens) {
		return arbToken{Type: arbTokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *arbitrageParser) consume() arbToken {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *arbitrageParser) parseExpression(minPrec int) (*arbNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		op := p.peek()
		prec := binaryPrecedence(op.Type)
		if prec < minPrec {
			break
		}
		p.consume()
		right, err := p.parseExpression(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &arbNode{
			Kind:  arbNodeBinary,
			Op:    op.Type,
			Left:  left,
			Right: right,
		}
	}
	return left, nil
}

func (p *arbitrageParser) parseUnary() (*arbNode, error) {
	tok := p.peek()
	if tok.Type == arbTokenPlus || tok.Type == arbTokenMinus {
		p.consume()
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &arbNode{
			Kind:  arbNodeUnary,
			Op:    tok.Type,
			Child: child,
		}, nil
	}
	return p.parsePrimary()
}

func (p *arbitrageParser) parsePrimary() (*arbNode, error) {
	tok := p.consume()
	switch tok.Type {
	case arbTokenNumber:
		return &arbNode{
			Kind:   arbNodeNumber,
			Number: tok.Value,
		}, nil
	case arbTokenIdent:
		return &arbNode{
			Kind:         arbNodeContract,
			Contract:     tok.Text,
			ContractNorm: normalizeToken(tok.Text),
		}, nil
	case arbTokenLParen:
		expr, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		next := p.consume()
		if next.Type != arbTokenRParen {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return expr, nil
	case arbTokenEOF:
		return nil, fmt.Errorf("unexpected end of formula")
	default:
		return nil, fmt.Errorf("unexpected token %q", tok.Text)
	}
}

func binaryPrecedence(t arbTokenType) int {
	switch t {
	case arbTokenPlus, arbTokenMinus:
		return 1
	case arbTokenMul, arbTokenDiv:
		return 2
	default:
		return -1
	}
}

func collectArbitrageContracts(node *arbNode, out map[string]struct{}) {
	if node == nil {
		return
	}
	switch node.Kind {
	case arbNodeContract:
		if node.ContractNorm != "" {
			out[node.ContractNorm] = struct{}{}
		}
	case arbNodeUnary:
		collectArbitrageContracts(node.Child, out)
	case arbNodeBinary:
		collectArbitrageContracts(node.Left, out)
		collectArbitrageContracts(node.Right, out)
	}
}

func evaluateArbitrageExpression(expr *compiledArbitrageExpr, prices map[string]float64) arbEvalResult {
	if expr == nil || expr.Root == nil {
		return arbEvalResult{Err: fmt.Errorf("formula is not compiled"), HasError: true}
	}
	outcome, err := evalArbitrageNode(expr.Root, prices)
	if err != nil {
		return arbEvalResult{
			Known:    false,
			Missing:  sortedMissingContracts(outcome.missing),
			Err:      err,
			HasError: true,
		}
	}
	return arbEvalResult{
		Value:   outcome.value,
		Known:   outcome.known,
		Missing: sortedMissingContracts(outcome.missing),
	}
}

type arbEvalState struct {
	value   float64
	known   bool
	missing map[string]struct{}
}

func evalArbitrageNode(node *arbNode, prices map[string]float64) (arbEvalState, error) {
	switch node.Kind {
	case arbNodeNumber:
		return arbEvalState{value: node.Number, known: true, missing: map[string]struct{}{}}, nil
	case arbNodeContract:
		if node.ContractNorm != "" {
			if value, ok := prices[node.ContractNorm]; ok {
				return arbEvalState{value: value, known: true, missing: map[string]struct{}{}}, nil
			}
		}
		missing := map[string]struct{}{}
		if node.ContractNorm != "" {
			missing[node.ContractNorm] = struct{}{}
		}
		return arbEvalState{known: false, missing: missing}, nil
	case arbNodeUnary:
		child, err := evalArbitrageNode(node.Child, prices)
		if err != nil {
			return child, err
		}
		if !child.known {
			return child, nil
		}
		if node.Op == arbTokenMinus {
			child.value = -child.value
		}
		return child, nil
	case arbNodeBinary:
		left, err := evalArbitrageNode(node.Left, prices)
		if err != nil {
			return left, err
		}
		right, err := evalArbitrageNode(node.Right, prices)
		if err != nil {
			return right, err
		}
		missing := mergeMissingContracts(left.missing, right.missing)
		if node.Op == arbTokenDiv && right.known && math.Abs(right.value) < 1e-12 {
			return arbEvalState{known: false, missing: missing}, fmt.Errorf("division by zero")
		}
		if !left.known || !right.known {
			return arbEvalState{known: false, missing: missing}, nil
		}
		state := arbEvalState{known: true, missing: missing}
		switch node.Op {
		case arbTokenPlus:
			state.value = left.value + right.value
		case arbTokenMinus:
			state.value = left.value - right.value
		case arbTokenMul:
			state.value = left.value * right.value
		case arbTokenDiv:
			state.value = left.value / right.value
		default:
			return arbEvalState{known: false, missing: missing}, fmt.Errorf("unsupported operator")
		}
		return state, nil
	default:
		return arbEvalState{known: false, missing: map[string]struct{}{}}, fmt.Errorf("unsupported expression node")
	}
}

func mergeMissingContracts(left, right map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		result[key] = struct{}{}
	}
	for key := range right {
		result[key] = struct{}{}
	}
	return result
}

func sortedMissingContracts(missing map[string]struct{}) []string {
	if len(missing) == 0 {
		return nil
	}
	out := make([]string, 0, len(missing))
	for key := range missing {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func buildMarketPriceMap(rows []MarketRow, valueFn func(MarketRow) string) map[string]float64 {
	out := make(map[string]float64, len(rows))
	for _, row := range rows {
		symbol := normalizeToken(row.Symbol)
		if symbol == "" {
			continue
		}
		value, ok := parseFloat(valueFn(row))
		if !ok {
			continue
		}
		out[symbol] = value
	}
	return out
}

func buildRawMarketPriceMap(rows []map[string]any, key string) map[string]float64 {
	out := make(map[string]float64, len(rows))
	if strings.TrimSpace(key) == "" {
		return out
	}
	for _, row := range rows {
		symbol := normalizeToken(asString(row["ctp_contract"]))
		if symbol == "" {
			continue
		}
		value, ok := asOptionalFloat(row[key])
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		out[symbol] = value
	}
	return out
}

func buildMarketLastPriceMap(rows []MarketRow) map[string]float64 {
	return buildMarketPriceMap(rows, func(row MarketRow) string { return row.Last })
}

func buildMarketOpenPriceMap(rows []MarketRow) map[string]float64 {
	return buildMarketPriceMap(rows, func(row MarketRow) string { return row.Open })
}

func buildMarketHighPriceMap(rows []MarketRow) map[string]float64 {
	return buildMarketPriceMap(rows, func(row MarketRow) string { return row.High })
}

func buildMarketLowPriceMap(rows []MarketRow) map[string]float64 {
	return buildMarketPriceMap(rows, func(row MarketRow) string { return row.Low })
}

func buildMarketPreClosePriceMap(rows []MarketRow) map[string]float64 {
	return buildMarketPriceMap(rows, func(row MarketRow) string { return row.PreClose })
}

func buildMarketPreSettlePriceMap(rows []MarketRow) map[string]float64 {
	return buildMarketPriceMap(rows, func(row MarketRow) string { return row.PreSettle })
}

func buildRawMarketLastPriceMap(rows []map[string]any) map[string]float64 {
	return buildRawMarketPriceMap(rows, "last")
}

func buildRawMarketHighPriceMap(rows []map[string]any) map[string]float64 {
	return buildRawMarketPriceMap(rows, "high")
}

func buildRawMarketLowPriceMap(rows []map[string]any) map[string]float64 {
	return buildRawMarketPriceMap(rows, "low")
}

func buildRawMarketOpenPriceMap(rows []map[string]any) map[string]float64 {
	return buildRawMarketPriceMap(rows, "open")
}

func buildRawMarketPreClosePriceMap(rows []map[string]any) map[string]float64 {
	return buildRawMarketPriceMap(rows, "pre_close")
}

func buildRawMarketPreSettlePriceMap(rows []map[string]any) map[string]float64 {
	return buildRawMarketPriceMap(rows, "pre_settlement")
}
