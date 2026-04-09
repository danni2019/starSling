package tui

import (
	"testing"

	"github.com/rivo/tview"
)

func TestSetupOutputAppendsChunks(t *testing.T) {
	ui := &UI{
		app: tview.NewApplication(),
	}
	ui.buildSetupScreen()

	ui.setSetupOutputText("")
	ui.appendSetupOutputChunk("Downloading Python...\n")
	ui.appendSetupOutputChunk("Installing requirements...\n")

	got := ui.setupOutput.GetText(false)
	if got != "Downloading Python...\nInstalling requirements...\n" {
		t.Fatalf("setup output = %q", got)
	}
}

func TestFinishBootstrapSetsFinalOutput(t *testing.T) {
	ui := &UI{
		app:         tview.NewApplication(),
		setupOutput: tview.NewTextView(),
		setupStatus: tview.NewTextView(),
	}

	ui.finishBootstrap("Python runtime ready\n", nil)

	if got := ui.setupOutput.GetText(false); got != "Python runtime ready\n" {
		t.Fatalf("setup output = %q", got)
	}
	if got := ui.setupStatus.GetText(false); got != "Completed." {
		t.Fatalf("setup status = %q", got)
	}
}
