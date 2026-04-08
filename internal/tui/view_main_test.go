package tui

import (
	"path/filepath"
	"testing"

	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
)

func TestOpenLiveScreenFromMainBlocksPlaceholderConfig(t *testing.T) {
	origBundledPythonPathFn := bundledPythonPathFn
	bundledPythonPathFn = func() string { return "/tmp/starsling-test-python" }
	defer func() { bundledPythonPathFn = origBundledPythonPathFn }()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	ui := testMainMenuUI()
	if _, err := configstore.Ensure(); err != nil {
		t.Fatalf("ensure configstore: %v", err)
	}

	ui.openLiveScreenFromMain()

	if ui.currentScreen() != screenMain {
		t.Fatalf("expected to remain on main screen, got %s", ui.currentScreen())
	}
	if !ui.pages.HasPage("live-config-required") {
		t.Fatalf("expected live-config-required modal to be shown")
	}
}

func TestOpenLiveScreenFromMainRedirectsToSetupWhenRuntimeMissing(t *testing.T) {
	origBundledPythonPathFn := bundledPythonPathFn
	bundledPythonPathFn = func() string { return "" }
	defer func() { bundledPythonPathFn = origBundledPythonPathFn }()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	ui := testMainMenuUI()

	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.LiveMD.Host = "127.0.0.1"
	cfg.LiveMD.Port = 4123
	if err := configstore.Save(configstore.DefaultName(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := configstore.SetDefault(configstore.DefaultName()); err != nil {
		t.Fatalf("set default config: %v", err)
	}

	ui.openLiveScreenFromMain()

	if ui.currentScreen() != screenMain {
		t.Fatalf("expected to remain on main screen before setup confirmation, got %s", ui.currentScreen())
	}
	if !ui.pages.HasPage("runtime-bootstrap-required") {
		t.Fatalf("expected runtime-bootstrap-required modal to be shown")
	}
}

func TestOpenLiveScreenFromMainAllowsConfiguredConfig(t *testing.T) {
	origBundledPythonPathFn := bundledPythonPathFn
	bundledPythonPathFn = func() string { return "/tmp/starsling-test-python" }
	defer func() { bundledPythonPathFn = origBundledPythonPathFn }()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	ui := testMainMenuUI()

	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.LiveMD.Host = "127.0.0.1"
	cfg.LiveMD.Port = 4123
	if err := configstore.Save(configstore.DefaultName(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := configstore.SetDefault(configstore.DefaultName()); err != nil {
		t.Fatalf("set default config: %v", err)
	}

	ui.openLiveScreenFromMain()

	if ui.currentScreen() != screenLive {
		t.Fatalf("expected to enter live screen, got %s", ui.currentScreen())
	}
	if ui.pages.HasPage("live-config-required") {
		t.Fatalf("did not expect live-config-required modal")
	}
}

func TestFinishBootstrapResumesLiveFlow(t *testing.T) {
	origBundledPythonPathFn := bundledPythonPathFn
	runtimeReady := false
	bundledPythonPathFn = func() string {
		if runtimeReady {
			return "/tmp/starsling-test-python"
		}
		return ""
	}
	defer func() { bundledPythonPathFn = origBundledPythonPathFn }()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	ui := testMainMenuUI()

	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.LiveMD.Host = "127.0.0.1"
	cfg.LiveMD.Port = 4123
	if err := configstore.Save(configstore.DefaultName(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := configstore.SetDefault(configstore.DefaultName()); err != nil {
		t.Fatalf("set default config: %v", err)
	}

	ui.openSetupScreen(false, true)
	runtimeReady = true
	ui.finishBootstrap("Python runtime ready", nil)

	if ui.currentScreen() != screenLive {
		t.Fatalf("expected bootstrap success to resume live flow, got %s", ui.currentScreen())
	}
	if ui.setupResumeLive {
		t.Fatalf("expected setupResumeLive to be cleared after successful resume")
	}
}

func TestMaybePromptRuntimeBootstrapOnStartupShowsModal(t *testing.T) {
	origBundledPythonPathFn := bundledPythonPathFn
	bundledPythonPathFn = func() string { return "" }
	defer func() { bundledPythonPathFn = origBundledPythonPathFn }()

	ui := testMainMenuUI()
	ui.maybePromptRuntimeBootstrapOnStartup()

	if !ui.pages.HasPage("runtime-bootstrap-required") {
		t.Fatalf("expected runtime-bootstrap-required modal on startup")
	}
	if !ui.startupRuntimePrompted {
		t.Fatalf("expected startupRuntimePrompted to be recorded")
	}
}

func testMainMenuUI() *UI {
	ui := &UI{
		app:   tview.NewApplication(),
		pages: tview.NewPages(),
	}

	main := ui.buildMainScreen()
	setupView := ui.buildSetupScreen()
	configView := ui.buildConfigScreen()
	ui.pages.AddPage(string(screenMain), main, true, true)
	ui.pages.AddPage(string(screenLive), tview.NewBox(), true, false)
	ui.pages.AddPage(string(screenSetup), setupView, true, false)
	ui.pages.AddPage(string(screenConfig), configView, true, false)
	ui.app.SetRoot(ui.pages, true)
	ui.setCurrentScreen(screenMain)
	ui.app.SetFocus(ui.menu)

	return ui
}
