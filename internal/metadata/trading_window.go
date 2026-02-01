package metadata

import (
	"encoding/json"
	"fmt"
	"time"
)

const preOpenLead = 30 * time.Minute

type TradeSegment struct {
	Start time.Duration
	End   time.Duration
}

type tradeTimeResponse struct {
	RspCode int            `json:"rsp_code"`
	Data    []tradeTimeRow `json:"data"`
	Message string         `json:"rsp_message"`
}

type tradeTimeRow struct {
	TimeBegin string `json:"TimeBegin"`
	TimeEnd   string `json:"TimeEnd"`
}

func LoadTradeSegments() ([]TradeSegment, error) {
	cached, err := Load("trade_time")
	if err != nil {
		return nil, err
	}
	var resp tradeTimeResponse
	if err := json.Unmarshal(cached.Data, &resp); err != nil {
		return nil, fmt.Errorf("parse trade_time data: %w", err)
	}
	if resp.RspCode != 0 {
		return nil, fmt.Errorf("trade_time rsp_code=%d", resp.RspCode)
	}
	segments := make([]TradeSegment, 0, len(resp.Data))
	for _, row := range resp.Data {
		start, err := parseTimeOfDay(row.TimeBegin)
		if err != nil {
			continue
		}
		end, err := parseTimeOfDay(row.TimeEnd)
		if err != nil {
			continue
		}
		segments = append(segments, TradeSegment{Start: start, End: end})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("no trade_time segments parsed")
	}
	return segments, nil
}

func InTradingWindow(now time.Time, segments []TradeSegment) bool {
	if len(segments) == 0 {
		return true
	}
	for _, seg := range segments {
		if inSegment(now, seg) {
			return true
		}
	}
	return false
}

func inSegment(now time.Time, seg TradeSegment) bool {
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if withinWindow(now, base, seg) {
		return true
	}
	return withinWindow(now, base.Add(-24*time.Hour), seg)
}

func withinWindow(now time.Time, base time.Time, seg TradeSegment) bool {
	startAdj := seg.Start - preOpenLead
	start := base.Add(startAdj)
	end := base.Add(seg.End)
	if seg.End < seg.Start {
		end = end.Add(24 * time.Hour)
	}
	return !now.Before(start) && !now.After(end)
}

func parseTimeOfDay(value string) (time.Duration, error) {
	parsed, err := time.Parse("15:04:05", value)
	if err != nil {
		return 0, err
	}
	return time.Duration(parsed.Hour())*time.Hour +
		time.Duration(parsed.Minute())*time.Minute +
		time.Duration(parsed.Second())*time.Second, nil
}
