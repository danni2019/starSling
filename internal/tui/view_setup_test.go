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

func TestSetupOutputNormalizesCarriageReturnProgress(t *testing.T) {
	ui := &UI{
		app: tview.NewApplication(),
	}
	ui.buildSetupScreen()

	ui.setSetupOutputText("")
	ui.appendSetupOutputChunk("Downloading Python...\r")
	ui.appendSetupOutputChunk("#####                     20.0%\r")
	ui.appendSetupOutputChunk("##########                40.0%\r")
	ui.appendSetupOutputChunk("done\n")

	got := ui.setupOutput.GetText(false)
	want := "Downloading Python...\n#####                     20.0%\n##########                40.0%\ndone\n"
	if got != want {
		t.Fatalf("setup output = %q, want %q", got, want)
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

func TestNormalizeProgressOutputTextConvertsCarriageReturns(t *testing.T) {
	raw := "Downloading...\r50%\r100%\nready\r\n"
	got := normalizeProgressOutputText(raw)
	want := "Downloading...\n50%\n100%\nready\n"
	if got != want {
		t.Fatalf("normalized output = %q, want %q", got, want)
	}
}
