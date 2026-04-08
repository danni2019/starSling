package tui

import (
	"strings"

	"github.com/danni2019/starSling/internal/live"
	internalruntime "github.com/danni2019/starSling/internal/runtime"
)

var (
	bundledPythonPathFn = live.BundledPythonPath
	runBootstrapFn      = internalruntime.RunBootstrap
)

func runtimeBootstrapNeeded() bool {
	return strings.TrimSpace(bundledPythonPathFn()) == ""
}

func (ui *UI) openSetupScreen(autoStart bool, resumeLive bool) {
	ui.setupAutoStart = autoStart
	ui.setupResumeLive = resumeLive
	ui.setScreen(screenSetup)
}

func (ui *UI) maybePromptRuntimeBootstrapOnStartup() {
	if ui == nil || ui.startupRuntimePrompted || !runtimeBootstrapNeeded() || ui.currentScreen() != screenMain {
		return
	}
	ui.startupRuntimePrompted = true
	ui.promptRuntimeBootstrapRequired(
		"Local Python runtime is not initialized yet.\n\nOpen Setup to download and prepare the runtime before using Live Market Data.\n\nYou can also return later from the main menu.",
		false,
	)
}
