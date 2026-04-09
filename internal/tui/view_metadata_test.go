package tui

import (
	"errors"
	"testing"

	"github.com/rivo/tview"
)

func TestMetadataOutputNormalizesCarriageReturnProgress(t *testing.T) {
	ui := &UI{
		app: tview.NewApplication(),
	}
	ui.buildMetadataScreen()

	ui.setMetadataOutputText("")
	ui.appendMetadataOutputChunk("Refreshing metadata...\r")
	ui.appendMetadataOutputChunk("contract updated\r")
	ui.appendMetadataOutputChunk("trade_time updated\n")

	got := ui.metadataOutput.GetText(false)
	want := "Refreshing metadata...\ncontract updated\ntrade_time updated\n"
	if got != want {
		t.Fatalf("metadata output = %q, want %q", got, want)
	}
}

func TestFinishMetadataRefreshSetsFinalOutputAndError(t *testing.T) {
	ui := &UI{
		app:            tview.NewApplication(),
		metadataOutput: tview.NewTextView(),
		metadataStatus: tview.NewTextView(),
	}

	ui.finishMetadataRefresh("refresh failed\n", errors.New("network unavailable"))

	if got := ui.metadataOutput.GetText(false); got != "refresh failed\n" {
		t.Fatalf("metadata output = %q", got)
	}
	if got := ui.metadataStatus.GetText(false); got != "Error: network unavailable" {
		t.Fatalf("metadata status = %q", got)
	}
}
