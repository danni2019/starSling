package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"
)

type arbMonitorState struct {
	ID         string
	Name       string
	Formula    string
	Compiled   *compiledArbitrageExpr
	CompileErr string

	LastValue         float64
	LastKnown         bool
	LastStatus        string
	LastMissing       []string
	LastEvalAt        time.Time
	OpenValue         float64
	OpenCaptured      bool
	HighValue         float64
	HighKnown         bool
	LowValue          float64
	LowKnown          bool
	PreCloseValue     float64
	PreCloseCaptured  bool
	PreSettleValue    float64
	PreSettleCaptured bool
}

func (ui *UI) setArbitrageFormula(raw string) {
	formula := strings.TrimSpace(raw)
	if formula == "" {
		ui.arbMonitors = nil
		ui.arbSelectedMonitorID = ""
		return
	}
	id := ui.newArbitrageMonitorID()
	monitor := arbMonitorState{
		ID:      id,
		Name:    "",
		Formula: formula,
	}
	ui.compileArbitrageMonitor(&monitor)
	ui.arbMonitors = []arbMonitorState{monitor}
	ui.arbSelectedMonitorID = id
}

func (ui *UI) openArbitrageMonitorSettings() {
	ui.openArbitrageMonitorManager(ui.arbSelectedMonitorID, "")
}

func (ui *UI) openArbitrageMonitorManager(preferredSelectedID, message string) {
	ui.setCurrentScreen(screenDrilldown)
	ui.normalizeArbitrageSelection()

	ids, labels, selectedIdx := ui.arbitrageMonitorOptionLabels()
	selectedID := strings.TrimSpace(preferredSelectedID)
	if ui.findArbitrageMonitorIndex(selectedID) < 0 {
		selectedID = ""
	}
	if selectedID == "" && selectedIdx >= 0 && selectedIdx < len(ids) {
		selectedID = ids[selectedIdx]
	}

	existingDrop := tview.NewDropDown().SetLabel("Existing pair: ")
	if len(labels) == 0 {
		existingDrop.SetOptions([]string{"(none)"}, nil)
		existingDrop.SetCurrentOption(0)
		existingDrop.SetDisabled(true)
	} else {
		if selectedID != "" {
			if idx := indexOfFold(ids, selectedID); idx >= 0 {
				selectedIdx = idx
			}
		}
		existingDrop.SetOptions(labels, func(_ string, index int) {
			if index < 0 || index >= len(ids) {
				return
			}
			selectedID = ids[index]
			ui.arbSelectedMonitorID = selectedID
		})
		existingDrop.SetCurrentOption(selectedIdx)
	}

	hint := tview.NewTextView().
		SetTextColor(colorMuted).
		SetText("Use New/Edit/Delete to manage arbitrage pairs.")
	if strings.TrimSpace(message) != "" {
		hint.SetText(strings.TrimSpace(message))
	}

	form := tview.NewForm().
		AddFormItem(existingDrop)
	form.SetBorder(true).SetTitle("Arbitrage Monitor Manager")
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)

	form.AddButton("New", func() {
		ui.openArbitragePairEditor("", "", "", selectedID)
	})
	form.AddButton("Edit", func() {
		idx := ui.findArbitrageMonitorIndex(selectedID)
		if idx < 0 {
			hint.SetText("no existing pair selected")
			return
		}
		monitor := ui.arbMonitors[idx]
		ui.openArbitragePairEditor(monitor.ID, monitor.Name, monitor.Formula, monitor.ID)
	})
	form.AddButton("Delete", func() {
		idx := ui.findArbitrageMonitorIndex(selectedID)
		if idx < 0 {
			hint.SetText("no existing pair selected")
			return
		}
		monitor := ui.arbMonitors[idx]
		ui.openArbitragePairDeleteConfirm(monitor.ID, monitor.Name, monitor.Formula, selectedID)
	})
	form.AddButton("Close", func() {
		ui.closeDrilldown()
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(hint, 1, 0, false)
	ui.pages.AddPage(string(screenDrilldown), centerModal(layout, 92, 12), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) openArbitragePairEditor(editID, currentName, currentFormula, returnSelectedID string) {
	ui.setCurrentScreen(screenDrilldown)

	nameInput := tview.NewInputField().
		SetLabel("Name(optional): ").
		SetText(strings.TrimSpace(currentName))
	formulaInput := tview.NewInputField().
		SetLabel("Formula: ").
		SetText(strings.TrimSpace(currentFormula))
	hint := tview.NewTextView().
		SetTextColor(colorMuted).
		SetText("Supports + - * / () and constants. Quote contracts with '-' using 'contract-name'.")

	form := tview.NewForm().
		AddFormItem(nameInput).
		AddFormItem(formulaInput)
	title := "Create Arbitrage Pair"
	if strings.TrimSpace(editID) != "" {
		title = "Edit Arbitrage Pair"
	}
	form.SetBorder(true).SetTitle(title)
	form.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	form.SetBackgroundColor(colorBackground)
	form.SetFieldBackgroundColor(colorBackground)
	form.SetFieldTextColor(colorTableRow)
	form.SetButtonBackgroundColor(colorHighlight)
	form.SetButtonTextColor(colorMenuSelected)

	form.AddButton("Apply", func() {
		if err := ui.upsertArbitrageMonitor(editID, nameInput.GetText(), formulaInput.GetText()); err != nil {
			hint.SetText("invalid pair: " + err.Error())
			return
		}
		ui.renderArbitrageMonitor()
		ui.saveArbitrageSettingsToStore()
		ui.openArbitrageMonitorManager(ui.arbSelectedMonitorID, "pair saved")
	})
	form.AddButton("Cancel", func() {
		ui.openArbitrageMonitorManager(returnSelectedID, "")
	})

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(hint, 1, 0, false)
	ui.pages.AddPage(string(screenDrilldown), centerModal(layout, 102, 13), true, true)
	ui.app.SetFocus(form)
}

func (ui *UI) openArbitragePairDeleteConfirm(id, name, formula, returnSelectedID string) {
	ui.setCurrentScreen(screenDrilldown)
	label := strings.TrimSpace(name)
	if label == "" {
		label = strings.TrimSpace(id)
	}
	if label == "" {
		label = strings.TrimSpace(formula)
	}
	if label == "" {
		label = "selected pair"
	}
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Delete arbitrage pair?\n%s", label)).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(_ int, buttonLabel string) {
			if buttonLabel == "Delete" {
				ui.deleteArbitrageMonitor(id)
				ui.renderArbitrageMonitor()
				ui.saveArbitrageSettingsToStore()
				ui.openArbitrageMonitorManager(ui.arbSelectedMonitorID, "pair deleted")
				return
			}
			ui.openArbitrageMonitorManager(returnSelectedID, "")
		})
	ui.pages.AddPage(string(screenDrilldown), centerModal(modal, 60, 10), true, true)
	ui.app.SetFocus(modal)
}

func (ui *UI) renderArbitrageMonitor() {
	if ui.liveFlow == nil || !ui.useArbMonitor {
		return
	}
	ui.normalizeArbitrageSelection()
	ui.liveFlow.SetTitle(fmt.Sprintf("Arbitrage Monitor (pairs: %d)", len(ui.arbMonitors)))
	if len(ui.arbMonitors) == 0 {
		fillArbitrageTable(ui.liveFlow, nil)
		return
	}
	lastPrices := buildMarketLastPriceMap(ui.marketRows)
	openPrices := buildMarketOpenPriceMap(ui.marketRows)
	highPrices := buildMarketHighPriceMap(ui.marketRows)
	lowPrices := buildMarketLowPriceMap(ui.marketRows)
	preClosePrices := buildMarketPreClosePriceMap(ui.marketRows)
	preSettlePrices := buildMarketPreSettlePriceMap(ui.marketRows)
	rows := make([]ArbitrageMonitorRow, 0, len(ui.arbMonitors))
	for idx := range ui.arbMonitors {
		monitor := &ui.arbMonitors[idx]
		status := "IDLE"
		valueText := "-"
		highText := "-"
		lowText := "-"
		openText := "-"
		preCloseText := "-"
		preSettleText := "-"
		missingText := "-"
		updatedText := "-"
		formula := strings.TrimSpace(monitor.Formula)
		if formula == "" {
			status = "INVALID"
			missingText = "empty formula"
		} else if monitor.Compiled == nil {
			status = "INVALID"
			missingText = defaultDash(monitor.CompileErr)
		} else {
			result := evaluateArbitrageExpression(monitor.Compiled, lastPrices)
			status = "PARTIAL"
			missingText = defaultDash(strings.Join(result.Missing, ","))
			if result.HasError {
				status = "RUNTIME_ERR"
				errText := "evaluation error"
				if result.Err != nil {
					errText = result.Err.Error()
				}
				if len(result.Missing) > 0 {
					missingText = fmt.Sprintf("%s | missing:%s", errText, strings.Join(result.Missing, ","))
				} else {
					missingText = errText
				}
			} else if result.Known {
				status = "READY"
				valueText = formatFloat(result.Value)
				missingText = "-"
			}
			highResult := evaluateArbitrageExpression(monitor.Compiled, highPrices)
			monitor.HighKnown = highResult.Known && !highResult.HasError
			if monitor.HighKnown {
				monitor.HighValue = highResult.Value
				highText = formatFloat(monitor.HighValue)
			}
			lowResult := evaluateArbitrageExpression(monitor.Compiled, lowPrices)
			monitor.LowKnown = lowResult.Known && !lowResult.HasError
			if monitor.LowKnown {
				monitor.LowValue = lowResult.Value
				lowText = formatFloat(monitor.LowValue)
			}
			if !monitor.OpenCaptured {
				openResult := evaluateArbitrageExpression(monitor.Compiled, openPrices)
				if openResult.Known && !openResult.HasError {
					monitor.OpenValue = openResult.Value
					monitor.OpenCaptured = true
				}
			}
			if monitor.OpenCaptured {
				openText = formatFloat(monitor.OpenValue)
			}
			if !monitor.PreCloseCaptured {
				preCloseResult := evaluateArbitrageExpression(monitor.Compiled, preClosePrices)
				if preCloseResult.Known && !preCloseResult.HasError {
					monitor.PreCloseValue = preCloseResult.Value
					monitor.PreCloseCaptured = true
				}
			}
			if monitor.PreCloseCaptured {
				preCloseText = formatFloat(monitor.PreCloseValue)
			}
			if !monitor.PreSettleCaptured {
				preSettleResult := evaluateArbitrageExpression(monitor.Compiled, preSettlePrices)
				if preSettleResult.Known && !preSettleResult.HasError {
					monitor.PreSettleValue = preSettleResult.Value
					monitor.PreSettleCaptured = true
				}
			}
			if monitor.PreSettleCaptured {
				preSettleText = formatFloat(monitor.PreSettleValue)
			}
			monitor.LastKnown = result.Known && !result.HasError
			if monitor.LastKnown {
				monitor.LastValue = result.Value
			}
			monitor.LastStatus = status
			monitor.LastMissing = append([]string(nil), result.Missing...)
			monitor.LastEvalAt = time.Now()
			updatedText = monitor.LastEvalAt.Format("15:04:05")
		}
		rows = append(rows, ArbitrageMonitorRow{
			Name:      displayArbitrageMonitorName(*monitor),
			Value:     valueText,
			High:      highText,
			Low:       lowText,
			Open:      openText,
			PreClose:  preCloseText,
			PreSettle: preSettleText,
			Status:    status,
			Missing:   missingText,
			UpdatedAt: updatedText,
			Formula:   defaultDash(formula),
		})
	}
	fillArbitrageTable(ui.liveFlow, rows)
}

func displayArbitrageMonitorName(m arbMonitorState) string {
	name := strings.TrimSpace(m.Name)
	if name != "" {
		return name
	}
	if strings.TrimSpace(m.ID) != "" {
		return m.ID
	}
	return "-"
}

func (ui *UI) upsertArbitrageMonitor(editID, rawName, rawFormula string) error {
	formula := strings.TrimSpace(rawFormula)
	if formula == "" {
		return fmt.Errorf("formula cannot be empty")
	}
	name := strings.TrimSpace(rawName)
	compiled, err := compileArbitrageExpression(formula)
	if err != nil {
		return err
	}
	targetID := strings.TrimSpace(editID)
	if targetID == "" {
		id := ui.newArbitrageMonitorID()
		monitor := arbMonitorState{
			ID:      id,
			Name:    name,
			Formula: formula,
		}
		resetArbitrageMonitorRuntimeState(&monitor)
		monitor.Compiled = compiled
		ui.arbMonitors = append(ui.arbMonitors, monitor)
		ui.arbSelectedMonitorID = id
		return nil
	}
	idx := ui.findArbitrageMonitorIndex(targetID)
	if idx < 0 {
		return fmt.Errorf("selected pair does not exist")
	}
	ui.arbMonitors[idx].Name = name
	ui.arbMonitors[idx].Formula = formula
	ui.arbMonitors[idx].Compiled = compiled
	ui.arbMonitors[idx].CompileErr = ""
	resetArbitrageMonitorRuntimeState(&ui.arbMonitors[idx])
	ui.arbSelectedMonitorID = ui.arbMonitors[idx].ID
	return nil
}

func (ui *UI) deleteArbitrageMonitor(id string) bool {
	idx := ui.findArbitrageMonitorIndex(id)
	if idx < 0 {
		return false
	}
	next := make([]arbMonitorState, 0, len(ui.arbMonitors)-1)
	next = append(next, ui.arbMonitors[:idx]...)
	next = append(next, ui.arbMonitors[idx+1:]...)
	ui.arbMonitors = next
	ui.normalizeArbitrageSelection()
	return true
}

func (ui *UI) findArbitrageMonitorIndex(id string) int {
	target := normalizeToken(id)
	if target == "" {
		return -1
	}
	for idx := range ui.arbMonitors {
		if normalizeToken(ui.arbMonitors[idx].ID) == target {
			return idx
		}
	}
	return -1
}

func (ui *UI) compileArbitrageMonitor(monitor *arbMonitorState) {
	if monitor == nil {
		return
	}
	monitor.Formula = strings.TrimSpace(monitor.Formula)
	monitor.Name = strings.TrimSpace(monitor.Name)
	resetArbitrageMonitorRuntimeState(monitor)
	monitor.Compiled = nil
	monitor.CompileErr = ""
	if monitor.Formula == "" {
		monitor.CompileErr = "formula cannot be empty"
		return
	}
	compiled, err := compileArbitrageExpression(monitor.Formula)
	if err != nil {
		monitor.CompileErr = err.Error()
		return
	}
	monitor.Compiled = compiled
}

func resetArbitrageMonitorRuntimeState(monitor *arbMonitorState) {
	if monitor == nil {
		return
	}
	monitor.LastValue = 0
	monitor.LastKnown = false
	monitor.LastStatus = ""
	monitor.LastMissing = nil
	monitor.LastEvalAt = time.Time{}
	monitor.OpenValue = 0
	monitor.OpenCaptured = false
	monitor.HighValue = 0
	monitor.HighKnown = false
	monitor.LowValue = 0
	monitor.LowKnown = false
	monitor.PreCloseValue = 0
	monitor.PreCloseCaptured = false
	monitor.PreSettleValue = 0
	monitor.PreSettleCaptured = false
}

func (ui *UI) arbitrageMonitorOptionLabels() ([]string, []string, int) {
	ids := make([]string, 0, len(ui.arbMonitors))
	labels := make([]string, 0, len(ui.arbMonitors))
	selectedIdx := 0
	selectedIDNorm := normalizeToken(ui.arbSelectedMonitorID)
	for idx := range ui.arbMonitors {
		monitor := ui.arbMonitors[idx]
		ids = append(ids, monitor.ID)
		label := displayArbitrageMonitorName(monitor) + " | " + strings.TrimSpace(monitor.Formula)
		labels = append(labels, label)
		if selectedIDNorm != "" && normalizeToken(monitor.ID) == selectedIDNorm {
			selectedIdx = idx
		}
	}
	return ids, labels, selectedIdx
}

func (ui *UI) normalizeArbitrageSelection() {
	if len(ui.arbMonitors) == 0 {
		ui.arbSelectedMonitorID = ""
		return
	}
	if ui.findArbitrageMonitorIndex(ui.arbSelectedMonitorID) >= 0 {
		return
	}
	ui.arbSelectedMonitorID = ui.arbMonitors[0].ID
}

func (ui *UI) newArbitrageMonitorID() string {
	used := make(map[string]struct{}, len(ui.arbMonitors))
	for _, monitor := range ui.arbMonitors {
		id := normalizeToken(monitor.ID)
		if id != "" {
			used[id] = struct{}{}
		}
	}
	for {
		ui.arbIDCounter++
		candidate := fmt.Sprintf("arb-%03d", ui.arbIDCounter)
		if _, exists := used[normalizeToken(candidate)]; exists {
			continue
		}
		return candidate
	}
}

func (ui *UI) renderLiveLowerPanel() {
	if ui.useArbMonitor {
		ui.renderArbitrageMonitor()
		return
	}
	ui.renderFlowAggregation()
}
