package tui

import (
	"testing"

	"github.com/danni2019/starSling/internal/settingsstore"
)

func TestLivePersistedSettingsLoadGenerationInvalidation(t *testing.T) {
	ui := &UI{}

	first := ui.beginLivePersistedSettingsLoad()
	if first == 0 {
		t.Fatalf("expected non-zero load generation")
	}
	if !ui.isCurrentLivePersistedSettingsLoad(first) {
		t.Fatalf("expected first generation to be current")
	}

	second := ui.beginLivePersistedSettingsLoad()
	if second <= first {
		t.Fatalf("expected generation to increase: first=%d second=%d", first, second)
	}
	if ui.isCurrentLivePersistedSettingsLoad(first) {
		t.Fatalf("expected first generation to be stale after newer load")
	}
	if !ui.isCurrentLivePersistedSettingsLoad(second) {
		t.Fatalf("expected second generation to be current")
	}

	ui.invalidateLivePersistedSettingsLoad()
	if ui.isCurrentLivePersistedSettingsLoad(second) {
		t.Fatalf("expected invalidate to make second generation stale")
	}
}

func TestApplyLivePersistedSettingsLoadResultDefersWhileDrilldownOpen(t *testing.T) {
	ui := &UI{}
	seq := ui.beginLivePersistedSettingsLoad()
	ui.setCurrentScreen(screenDrilldown)
	ui.overviewSortBy = "turnover"

	cfg := settingsstore.Default()
	cfg.Overview.SortBy = "oi_chg"

	afterCalls := 0
	deferred := ui.applyLivePersistedSettingsLoadResult(seq, cfg, nil, func() {
		afterCalls++
	})
	if !deferred {
		t.Fatalf("expected settings apply to defer while drilldown is open")
	}
	if ui.overviewSortBy != "turnover" {
		t.Fatalf("expected UI state unchanged while deferred, got overviewSortBy=%q", ui.overviewSortBy)
	}
	if afterCalls != 0 {
		t.Fatalf("expected afterApply not to run while deferred, got %d calls", afterCalls)
	}

	ui.setCurrentScreen(screenLive)
	deferred = ui.applyLivePersistedSettingsLoadResult(seq, cfg, nil, func() {
		afterCalls++
	})
	if deferred {
		t.Fatalf("expected settings apply to complete on live screen")
	}
	if ui.overviewSortBy != "oi_chg" {
		t.Fatalf("expected persisted overview sort to apply, got %q", ui.overviewSortBy)
	}
	if afterCalls != 1 {
		t.Fatalf("expected afterApply to run once after successful apply, got %d calls", afterCalls)
	}
}

func TestRetryLivePersistedSettingsApplyReloadsFreshSettingsAfterDefer(t *testing.T) {
	ui := &UI{}
	seq := ui.beginLivePersistedSettingsLoad()
	ui.setCurrentScreen(screenDrilldown)
	ui.overviewSortBy = "turnover"

	staleCfg := settingsstore.Default()
	staleCfg.Overview.SortBy = "turnover"
	if deferred := ui.applyLivePersistedSettingsLoadResult(seq, staleCfg, nil, nil); !deferred {
		t.Fatalf("expected initial apply to defer while drilldown is open")
	}

	freshCfg := settingsstore.Default()
	freshCfg.Overview.SortBy = "oi_chg"

	origLoad := loadPersistedSettingsFn
	defer func() { loadPersistedSettingsFn = origLoad }()

	loadCalls := 0
	loadPersistedSettingsFn = func() (settingsstore.Settings, error) {
		loadCalls++
		return freshCfg, nil
	}

	ui.setCurrentScreen(screenLive)
	afterCalls := 0
	ui.retryLivePersistedSettingsApply(seq, func() {
		afterCalls++
	})

	if loadCalls != 1 {
		t.Fatalf("expected retry to reload settings once, got %d loads", loadCalls)
	}
	if ui.overviewSortBy != "oi_chg" {
		t.Fatalf("expected retry to apply fresh settings, got overviewSortBy=%q", ui.overviewSortBy)
	}
	if afterCalls != 1 {
		t.Fatalf("expected afterApply to run once on successful retry, got %d calls", afterCalls)
	}
}

func TestResumeDeferredLivePersistedSettingsApplyReloadsFreshSettingsOnce(t *testing.T) {
	ui := &UI{}
	seq := ui.beginLivePersistedSettingsLoad()
	ui.setCurrentScreen(screenDrilldown)
	ui.overviewSortBy = "turnover"

	staleCfg := settingsstore.Default()
	staleCfg.Overview.SortBy = "turnover"

	afterCalls := 0
	if deferred := ui.applyLivePersistedSettingsLoadResult(seq, staleCfg, nil, func() {
		afterCalls++
	}); !deferred {
		t.Fatalf("expected apply to defer while drilldown is open")
	}
	if !ui.liveSettingsApplyPending || ui.liveSettingsApplySeq != seq {
		t.Fatalf("expected deferred apply marker for seq=%d", seq)
	}

	freshCfg := settingsstore.Default()
	freshCfg.Overview.SortBy = "oi_chg"

	origLoad := loadPersistedSettingsFn
	defer func() { loadPersistedSettingsFn = origLoad }()

	loadCalls := 0
	loadPersistedSettingsFn = func() (settingsstore.Settings, error) {
		loadCalls++
		return freshCfg, nil
	}

	ui.setCurrentScreen(screenLive)
	ui.resumeDeferredLivePersistedSettingsApply()

	if loadCalls != 1 {
		t.Fatalf("expected resume to reload settings once, got %d loads", loadCalls)
	}
	if ui.overviewSortBy != "oi_chg" {
		t.Fatalf("expected resume to apply fresh settings, got overviewSortBy=%q", ui.overviewSortBy)
	}
	if afterCalls != 1 {
		t.Fatalf("expected deferred afterApply to run once after resume, got %d calls", afterCalls)
	}
	if ui.liveSettingsApplyPending {
		t.Fatalf("expected deferred apply marker to be cleared after resume")
	}

	ui.resumeDeferredLivePersistedSettingsApply()
	if loadCalls != 1 {
		t.Fatalf("expected second resume with no pending marker to do nothing, got %d loads", loadCalls)
	}
}
