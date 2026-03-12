package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/danni2019/starSling/internal/router"
	"github.com/danni2019/starSling/internal/settingsstore"
)

var loadPersistedSettingsFn = settingsstore.Load

func (ui *UI) beginLivePersistedSettingsLoad() uint64 {
	seq := ui.liveSettingsLoadSeq.Add(1)
	ui.clearDeferredLivePersistedSettingsApply()
	return seq
}

func (ui *UI) invalidateLivePersistedSettingsLoad() {
	ui.liveSettingsLoadSeq.Add(1)
	ui.clearDeferredLivePersistedSettingsApply()
}

func (ui *UI) isCurrentLivePersistedSettingsLoad(seq uint64) bool {
	return ui.liveSettingsLoadSeq.Load() == seq
}

func (ui *UI) loadLivePersistedSettingsAsync(afterApply func()) {
	seq := ui.beginLivePersistedSettingsLoad()
	go func() {
		cfg, err := loadPersistedSettingsFn()
		ui.queueLivePersistedSettingsApply(seq, cfg, err, afterApply)
	}()
}

func (ui *UI) queueLivePersistedSettingsApply(seq uint64, cfg settingsstore.Settings, loadErr error, afterApply func()) {
	if ui.app == nil {
		_ = ui.applyLivePersistedSettingsLoadResult(seq, cfg, loadErr, afterApply)
		return
	}
	ui.app.QueueUpdateDraw(func() {
		_ = ui.applyLivePersistedSettingsLoadResult(seq, cfg, loadErr, afterApply)
	})
}

func (ui *UI) retryLivePersistedSettingsApply(seq uint64, afterApply func()) {
	cfg, err := loadPersistedSettingsFn()
	ui.queueLivePersistedSettingsApply(seq, cfg, err, afterApply)
}

func (ui *UI) deferLivePersistedSettingsApply(seq uint64, afterApply func()) {
	ui.liveSettingsApplyPending = true
	ui.liveSettingsApplySeq = seq
	ui.liveSettingsApplyAfter = afterApply
}

func (ui *UI) clearDeferredLivePersistedSettingsApply() {
	ui.liveSettingsApplyPending = false
	ui.liveSettingsApplySeq = 0
	ui.liveSettingsApplyAfter = nil
}

func (ui *UI) resumeDeferredLivePersistedSettingsApply() {
	if !ui.liveSettingsApplyPending {
		return
	}
	seq := ui.liveSettingsApplySeq
	afterApply := ui.liveSettingsApplyAfter
	ui.clearDeferredLivePersistedSettingsApply()
	if !ui.isCurrentLivePersistedSettingsLoad(seq) {
		return
	}
	if ui.app == nil {
		ui.retryLivePersistedSettingsApply(seq, afterApply)
		return
	}
	go ui.retryLivePersistedSettingsApply(seq, afterApply)
}

func (ui *UI) applyLivePersistedSettingsLoadResult(seq uint64, cfg settingsstore.Settings, loadErr error, afterApply func()) bool {
	screen := ui.currentScreen()
	if (screen != screenLive && screen != screenDrilldown) || !ui.isCurrentLivePersistedSettingsLoad(seq) {
		return false
	}
	if loadErr != nil {
		ui.appendLiveLogLine("settings load warning: " + loadErr.Error())
		if afterApply != nil {
			afterApply()
		}
		return false
	}
	if screen == screenDrilldown {
		// Drilldown forms snapshot current UI values when they open. Applying persisted
		// settings underneath an open modal can desync the form widgets from UI state.
		ui.deferLivePersistedSettingsApply(seq, afterApply)
		return true
	}
	if ui.liveSettingsApplyPending && ui.liveSettingsApplySeq == seq {
		ui.clearDeferredLivePersistedSettingsApply()
	}
	ui.applyPersistedSettings(cfg)
	ui.refreshLivePanelsFromPersistedSettings()
	ui.pushUnusualThresholdsAsync(
		cfg.Unusual.TurnoverChgThreshold,
		cfg.Unusual.TurnoverRatioThreshold,
		cfg.Unusual.OIRatioThreshold,
	)
	ui.pushOverviewGammaBucketsAsync(cfg.GammaBucketFrontDays, cfg.GammaBucketMidDays)
	if afterApply != nil {
		afterApply()
	}
	return false
}

func (ui *UI) loadLivePersistedSettings() {
	cfg, err := loadPersistedSettingsFn()
	if err != nil {
		ui.appendLiveLogLine("settings load warning: " + err.Error())
		return
	}
	ui.applyPersistedSettings(cfg)
	ui.refreshLivePanelsFromPersistedSettings()
	ui.pushUnusualThresholdsAsync(
		cfg.Unusual.TurnoverChgThreshold,
		cfg.Unusual.TurnoverRatioThreshold,
		cfg.Unusual.OIRatioThreshold,
	)
	ui.pushOverviewGammaBucketsAsync(cfg.GammaBucketFrontDays, cfg.GammaBucketMidDays)
}

func (ui *UI) applyPersistedSettings(cfg settingsstore.Settings) {
	ui.overviewSortBy = cfg.Overview.SortBy
	ui.overviewSortAsc = cfg.Overview.SortAsc
	ui.overviewRequireOptions = cfg.Overview.RequireOptions

	ui.filterExchange = cfg.Market.Exchange
	ui.filterClass = cfg.Market.Class
	ui.filterSymbol = cfg.Market.Symbol
	ui.filterContract = cfg.Market.Contract
	ui.filterMainOnly = cfg.Market.MainOnly
	ui.marketSortBy = cfg.Market.SortBy
	ui.marketSortAsc = cfg.Market.SortAsc

	ui.optionsDeltaEnabled = cfg.Options.DeltaEnabled
	ui.optionsDeltaAbsMin = cfg.Options.DeltaAbsMin
	ui.optionsDeltaAbsMax = cfg.Options.DeltaAbsMax

	ui.unusualChgThreshold = cfg.Unusual.TurnoverChgThreshold
	ui.unusualRatioThreshold = cfg.Unusual.TurnoverRatioThreshold
	ui.unusualOIRatioThreshold = cfg.Unusual.OIRatioThreshold
	ui.unusualFilterSymbol = cfg.Unusual.Symbol
	ui.liveDisconnectTimeoutSeconds = cfg.LiveDisconnectTimeoutSeconds
	ui.liveProcessHangTimeoutSeconds = cfg.LiveProcessHangTimeoutSeconds

	selectedOnly, focusedOnly := normalizeExclusiveFlowFilters(cfg.Flow.OnlySelected, cfg.Flow.OnlyFocused)
	ui.flowOnlySelectedContracts = selectedOnly
	ui.flowOnlyFocusedSymbol = focusedOnly
	ui.flowWindowSeconds = cfg.Flow.WindowSeconds
	ui.flowMinAnalysisSeconds = cfg.Flow.MinAnalysisSeconds

	ui.applyArbitrageSettings(cfg.Arbitrage)
}

func (ui *UI) refreshLivePanelsFromPersistedSettings() {
	screen := ui.currentScreen()
	if screen != screenLive && screen != screenDrilldown {
		return
	}

	if ui.liveMarket != nil {
		ui.renderMarketRows()
	}
	ui.ensureFocusSymbol()

	if ui.liveOverview != nil {
		ui.renderOverviewPanels()
	}
	if ui.liveOpts != nil {
		ui.renderOptionsSnapshot()
	}
	if ui.liveTrades != nil {
		ui.renderUnusualTradesFromState()
	}
	if ui.liveFlow != nil {
		ui.renderLiveLowerPanel()
	}
}

func (ui *UI) persistSettingsMutation(mutator func(*settingsstore.Settings)) error {
	cfg, err := loadPersistedSettingsFn()
	if err != nil {
		// Proceed with defaults so user actions still save a valid file.
		ui.appendLiveLogLine("settings load warning: " + err.Error())
	}
	mutator(&cfg)
	return settingsstore.Save(cfg)
}

func (ui *UI) saveOverviewSettingsToStore() {
	err := ui.persistSettingsMutation(func(cfg *settingsstore.Settings) {
		cfg.Overview.SortBy = ui.overviewSortBy
		cfg.Overview.SortAsc = ui.overviewSortAsc
		cfg.Overview.RequireOptions = ui.overviewRequireOptions
	})
	if err != nil {
		ui.appendLiveLogLine("save overview settings failed: " + err.Error())
	}
}

func (ui *UI) saveMarketSettingsToStore() {
	err := ui.persistSettingsMutation(func(cfg *settingsstore.Settings) {
		cfg.Market.Exchange = strings.TrimSpace(ui.filterExchange)
		cfg.Market.Class = strings.TrimSpace(ui.filterClass)
		cfg.Market.Symbol = strings.TrimSpace(ui.filterSymbol)
		cfg.Market.Contract = strings.TrimSpace(ui.filterContract)
		cfg.Market.MainOnly = ui.filterMainOnly
		cfg.Market.SortBy = strings.TrimSpace(ui.marketSortBy)
		cfg.Market.SortAsc = ui.marketSortAsc
	})
	if err != nil {
		ui.appendLiveLogLine("save market settings failed: " + err.Error())
	}
}

func (ui *UI) saveOptionsSettingsToStore() {
	err := ui.persistSettingsMutation(func(cfg *settingsstore.Settings) {
		cfg.Options.DeltaEnabled = ui.optionsDeltaEnabled
		cfg.Options.DeltaAbsMin = ui.optionsDeltaAbsMin
		cfg.Options.DeltaAbsMax = ui.optionsDeltaAbsMax
	})
	if err != nil {
		ui.appendLiveLogLine("save options settings failed: " + err.Error())
	}
}

func (ui *UI) saveUnusualSettingsToStore() {
	err := ui.persistSettingsMutation(func(cfg *settingsstore.Settings) {
		cfg.Unusual.TurnoverChgThreshold = ui.unusualChgThreshold
		cfg.Unusual.TurnoverRatioThreshold = ui.unusualRatioThreshold
		cfg.Unusual.OIRatioThreshold = ui.unusualOIRatioThreshold
		cfg.Unusual.Symbol = strings.TrimSpace(ui.unusualFilterSymbol)
		cfg.Unusual.Contract = ""
	})
	if err != nil {
		ui.appendLiveLogLine("save unusual settings failed: " + err.Error())
	}
}

func (ui *UI) saveFlowSettingsToStore() {
	err := ui.persistSettingsMutation(func(cfg *settingsstore.Settings) {
		cfg.Flow.WindowSeconds = ui.flowWindowSeconds
		cfg.Flow.MinAnalysisSeconds = ui.flowMinAnalysisSeconds
		cfg.Flow.OnlySelected = ui.flowOnlySelectedContracts
		cfg.Flow.OnlyFocused = ui.flowOnlyFocusedSymbol
	})
	if err != nil {
		ui.appendLiveLogLine("save flow settings failed: " + err.Error())
	}
}

func (ui *UI) saveArbitrageSettingsToStore() {
	err := ui.persistSettingsMutation(func(cfg *settingsstore.Settings) {
		pairs := make([]settingsstore.SettingsArbitragePair, 0, len(ui.arbMonitors))
		for _, monitor := range ui.arbMonitors {
			formula := strings.TrimSpace(monitor.Formula)
			if formula == "" {
				continue
			}
			pairs = append(pairs, settingsstore.SettingsArbitragePair{
				ID:      strings.TrimSpace(monitor.ID),
				Name:    strings.TrimSpace(monitor.Name),
				Formula: formula,
			})
		}
		cfg.Arbitrage.Pairs = pairs
		if len(pairs) > 0 {
			cfg.Arbitrage.Formula = pairs[0].Formula
		} else {
			cfg.Arbitrage.Formula = ""
		}
	})
	if err != nil {
		ui.appendLiveLogLine("save arbitrage settings failed: " + err.Error())
	}
}

func (ui *UI) applyArbitrageSettings(arb settingsstore.SettingsArbitrage) {
	ui.arbMonitors = nil
	ui.arbSelectedMonitorID = ""
	ui.arbIDCounter = 0

	if len(arb.Pairs) > 0 {
		seen := make(map[string]struct{}, len(arb.Pairs))
		for _, pair := range arb.Pairs {
			formula := strings.TrimSpace(pair.Formula)
			if formula == "" {
				continue
			}
			id := strings.TrimSpace(pair.ID)
			if id == "" {
				id = ui.newArbitrageMonitorID()
			}
			key := normalizeToken(id)
			if key == "" {
				id = ui.newArbitrageMonitorID()
				key = normalizeToken(id)
			}
			if _, exists := seen[key]; exists {
				id = ui.newArbitrageMonitorID()
				key = normalizeToken(id)
			}
			seen[key] = struct{}{}
			monitor := arbMonitorState{
				ID:      id,
				Name:    strings.TrimSpace(pair.Name),
				Formula: formula,
			}
			ui.compileArbitrageMonitor(&monitor)
			ui.arbMonitors = append(ui.arbMonitors, monitor)
		}
		ui.normalizeArbitrageSelection()
		return
	}

	legacy := strings.TrimSpace(arb.Formula)
	if legacy == "" {
		return
	}
	ui.setArbitrageFormula(legacy)
}

func (ui *UI) pushOverviewGammaBuckets(frontDays, midDays int) error {
	if ui.rpcClient == nil {
		return fmt.Errorf("router rpc unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return ui.rpcClient.Call(ctx, "ui.set_overview_gamma_buckets", router.SetOverviewGammaBucketsParams{
		FrontDays: frontDays,
		MidDays:   midDays,
	}, nil)
}

func (ui *UI) pushOverviewGammaBucketsAsync(frontDays, midDays int) {
	if ui.rpcClient == nil {
		return
	}
	go func(front, mid int) {
		err := ui.pushOverviewGammaBuckets(front, mid)
		if err == nil {
			return
		}
		ui.app.QueueUpdateDraw(func() {
			ui.appendLiveLogLine("sync overview gamma buckets failed: " + err.Error())
		})
	}(frontDays, midDays)
}

func (ui *UI) pushUnusualThresholdsAsync(chgThreshold, ratioThreshold, oiRatioThreshold float64) {
	if ui.rpcClient == nil {
		return
	}
	go func(chg, ratio, oiRatio float64) {
		err := ui.pushUnusualThresholdsValues(chg, ratio, oiRatio)
		if err == nil {
			return
		}
		ui.app.QueueUpdateDraw(func() {
			ui.appendLiveLogLine("sync unusual thresholds failed: " + err.Error())
		})
	}(chgThreshold, ratioThreshold, oiRatioThreshold)
}

func (ui *UI) applyGlobalSettingsRuntime(frontDays, midDays int, restartOptionsWorker bool) {
	ui.pushOverviewGammaBucketsAsync(frontDays, midDays)
	if !restartOptionsWorker {
		return
	}
	if ui.liveProc != nil && !ui.liveProc.Done() {
		ui.appendLiveLogLine("restarting options worker to apply settings")
		ui.restartOptionsWorkerAfterExit()
		return
	}
	if ui.deferOptionsWorkerRestartUntilLiveStartupCompletes() {
		ui.appendLiveLogLine("deferring options worker restart until live startup completes")
	}
}
