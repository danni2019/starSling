package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rivo/tview"

	"github.com/danni2019/starSling/internal/configstore"
)

func (ui *UI) buildConfigScreen() tview.Primitive {
	ui.configStatus = tview.NewTextView()
	ui.configStatus.SetTextColor(colorMuted)
	ui.configStatus.SetBackgroundColor(colorBackground)
	ui.configStatus.SetBorder(true).SetTitle("Status")
	ui.configStatus.SetBorderColor(colorBorder).SetTitleColor(colorBorder)

	ui.configPages = tview.NewPages()
	ui.configMenu = buildConfigList("Config")
	ui.configMenu.AddItem("Use existing config", "", 0, func() {
		ui.showConfigSelect()
	})
	ui.configMenu.AddItem("Create or update config", "", 0, func() {
		ui.showConfigEdit()
	})
	ui.configMenu.AddItem("Delete config", "", 0, func() {
		ui.showConfigDelete()
	})
	ui.configMenu.AddItem("Back", "", 0, func() {
		ui.setScreen(screenMain)
	})

	ui.configSelect = buildConfigList("Select config")
	ui.configDelete = buildConfigList("Delete config")
	ui.configForm = tview.NewForm()
	ui.configForm.SetBorder(true).SetTitle("Edit config")
	ui.configForm.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.configForm.SetBackgroundColor(colorBackground)

	ui.configNameForm = tview.NewForm()
	ui.configNameForm.SetBorder(true).SetTitle("Save as new config")
	ui.configNameForm.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	ui.configNameForm.SetBackgroundColor(colorBackground)

	ui.configPages.AddPage("menu", ui.configMenu, true, true)
	ui.configPages.AddPage("select", ui.configSelect, true, false)
	ui.configPages.AddPage("delete", ui.configDelete, true, false)
	ui.configPages.AddPage("edit", ui.configForm, true, false)
	ui.configPages.AddPage("name", ui.configNameForm, true, false)

	help := tview.NewTextView()
	help.SetTextAlign(tview.AlignCenter)
	help.SetTextColor(colorMuted)
	help.SetBackgroundColor(colorBackground)
	help.SetText("Keys: Enter select  Esc back  q quit")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.configPages, 0, 1, true).
		AddItem(ui.configStatus, 3, 0, false).
		AddItem(help, 1, 0, false)
	layout.SetBackgroundColor(colorBackground)
	return layout
}

func buildConfigList(title string) *tview.List {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetMainTextColor(colorMenu)
	list.SetSelectedTextColor(colorMenuSelected)
	list.SetSelectedBackgroundColor(colorHighlight)
	list.SetBackgroundColor(colorBackground)
	list.SetBorder(true).SetTitle(title)
	list.SetBorderColor(colorBorder).SetTitleColor(colorBorder)
	return list
}

func (ui *UI) showConfigMenu() {
	if _, err := configstore.Ensure(); err != nil {
		ui.setConfigStatus(err.Error())
	}
	ui.switchConfigPage("menu", ui.configMenu)
}

func (ui *UI) showConfigSelect() {
	names, err := configstore.List()
	if err != nil {
		ui.setConfigStatus(err.Error())
		ui.switchConfigPage("menu", ui.configMenu)
		return
	}
	ui.configSelect.Clear()
	if len(names) == 0 {
		ui.configSelect.AddItem("No configs available", "", 0, nil)
	} else {
		for _, name := range names {
			configName := name
			ui.configSelect.AddItem(configName, "", 0, func() {
				if err := configstore.SetDefault(configName); err != nil {
					ui.setConfigStatus(err.Error())
				} else {
					ui.setConfigStatus(fmt.Sprintf("Default config set: %s", configName))
				}
				ui.showConfigMenu()
			})
		}
	}
	ui.configSelect.AddItem("Back", "", 0, func() {
		ui.showConfigMenu()
	})
	ui.switchConfigPage("select", ui.configSelect)
}

func (ui *UI) showConfigEdit() {
	name, cfg, err := configstore.LoadDefault()
	if err != nil {
		ui.setConfigStatus(err.Error())
		ui.showConfigMenu()
		return
	}
	ui.configEditingName = name
	ui.configEditingCfg = cfg

	ui.configForm.Clear(true)
	ui.configInputAPI = tview.NewInputField().SetLabel("API: ").SetText(cfg.LiveMD.API)
	ui.configInputProto = tview.NewInputField().SetLabel("Protocol: ").SetText(cfg.LiveMD.Protocol)
	ui.configInputHost = tview.NewInputField().SetLabel("Host: ").SetText(cfg.LiveMD.Host)
	ui.configInputPort = tview.NewInputField().SetLabel("Port: ").SetText(formatPort(cfg.LiveMD.Port))
	ui.configInputPort.SetAcceptanceFunc(acceptPort)
	ui.configInputUser = tview.NewInputField().SetLabel("Username: ").SetText(cfg.LiveMD.Username)
	ui.configInputPass = tview.NewInputField().SetLabel("Password: ").SetText(cfg.LiveMD.Password)
	ui.configInputPass.SetMaskCharacter('*')

	ui.configForm.AddFormItem(ui.configInputAPI)
	ui.configForm.AddFormItem(ui.configInputProto)
	ui.configForm.AddFormItem(ui.configInputHost)
	ui.configForm.AddFormItem(ui.configInputPort)
	ui.configForm.AddFormItem(ui.configInputUser)
	ui.configForm.AddFormItem(ui.configInputPass)

	ui.configForm.AddButton("Save as default", func() {
		ui.saveConfig(ui.configEditingName, true)
	})
	ui.configForm.AddButton("Save as new", func() {
		ui.showConfigNamePrompt()
	})
	ui.configForm.AddButton("Cancel", func() {
		ui.showConfigMenu()
	})

	ui.configForm.SetBorder(true).SetTitle(fmt.Sprintf("Edit config (%s)", name))
	ui.switchConfigPage("edit", ui.configForm)
}

func (ui *UI) showConfigNamePrompt() {
	ui.configNameForm.Clear(true)
	ui.configNameInput = tview.NewInputField().SetLabel("Config name: ")
	ui.configNameForm.AddFormItem(ui.configNameInput)
	ui.configNameForm.AddButton("Save", func() {
		name := strings.TrimSpace(ui.configNameInput.GetText())
		if name == "" {
			ui.setConfigStatus("config name is required")
			return
		}
		normalized, err := configstore.NormalizeName(name)
		if err != nil {
			ui.setConfigStatus(err.Error())
			return
		}
		exists, err := configstore.Exists(normalized)
		if err != nil {
			ui.setConfigStatus(err.Error())
			return
		}
		if exists {
			ui.setConfigStatus("config already exists; choose a new name")
			return
		}
		ui.saveConfig(normalized, false)
	})
	ui.configNameForm.AddButton("Cancel", func() {
		ui.switchConfigPage("edit", ui.configForm)
	})
	ui.switchConfigPage("name", ui.configNameForm)
}

func (ui *UI) showConfigDelete() {
	names, err := configstore.List()
	if err != nil {
		ui.setConfigStatus(err.Error())
		ui.showConfigMenu()
		return
	}
	ui.configDelete.Clear()
	if len(names) <= 1 {
		ui.configDelete.AddItem("Cannot delete last config", "", 0, nil)
	} else {
		for _, name := range names {
			configName := name
			ui.configDelete.AddItem(configName, "", 0, func() {
				ui.showDeleteConfirm(configName)
			})
		}
	}
	ui.configDelete.AddItem("Back", "", 0, func() {
		ui.showConfigMenu()
	})
	ui.switchConfigPage("delete", ui.configDelete)
}

func (ui *UI) showDeleteConfirm(name string) {
	ui.showModal("delete-confirm", fmt.Sprintf("Delete %q?\nThis cannot be undone.", name), []string{"Delete", "Cancel"}, func(index int, _ string) {
		if index == 0 {
			if err := configstore.Delete(name); err != nil {
				ui.setConfigStatus(err.Error())
			} else {
				ui.setConfigStatus(fmt.Sprintf("Deleted config: %s", name))
			}
		}
		ui.showConfigMenu()
	})
}

func (ui *UI) saveConfig(name string, setDefault bool) {
	cfg := ui.configEditingCfg
	cfg.LiveMD.API = strings.TrimSpace(ui.configInputAPI.GetText())
	cfg.LiveMD.Protocol = strings.TrimSpace(ui.configInputProto.GetText())
	cfg.LiveMD.Host = strings.TrimSpace(ui.configInputHost.GetText())
	cfg.LiveMD.Username = strings.TrimSpace(ui.configInputUser.GetText())
	cfg.LiveMD.Password = ui.configInputPass.GetText()

	portValue := strings.TrimSpace(ui.configInputPort.GetText())
	if portValue == "" {
		ui.setConfigStatus("port is required")
		return
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port <= 0 || port > 65535 {
		ui.setConfigStatus("port must be between 1 and 65535")
		return
	}
	cfg.LiveMD.Port = port

	if err := cfg.Validate(); err != nil {
		ui.setConfigStatus(err.Error())
		return
	}
	if err := configstore.Save(name, cfg); err != nil {
		ui.setConfigStatus(err.Error())
		return
	}
	if setDefault {
		if err := configstore.SetDefault(name); err != nil {
			ui.setConfigStatus(err.Error())
			return
		}
	}
	ui.setConfigStatus(fmt.Sprintf("Saved config: %s", name))
	ui.showConfigMenu()
}

func (ui *UI) switchConfigPage(name string, focus tview.Primitive) {
	ui.configPages.SwitchToPage(name)
	ui.app.SetFocus(focus)
}

func (ui *UI) setConfigStatus(message string) {
	ui.configStatus.SetText(strings.TrimSpace(message))
}

func formatPort(port int) string {
	if port <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", port)
}

func acceptPort(textToCheck string, _ rune) bool {
	if textToCheck == "" {
		return true
	}
	_, err := strconv.Atoi(textToCheck)
	return err == nil
}

func (ui *UI) showModal(name, message string, buttons []string, done func(index int, label string)) {
	modal := tview.NewModal()
	modal.SetText(message)
	modal.AddButtons(buttons)
	modal.SetTextColor(colorAccent)
	modal.SetBackgroundColor(colorBackground)
	modal.SetButtonBackgroundColor(colorHighlight)
	modal.SetButtonTextColor(colorMenuSelected)
	modal.SetDoneFunc(func(index int, label string) {
		ui.pages.RemovePage(name)
		done(index, label)
	})
	ui.pages.AddPage(name, modal, true, true)
	ui.app.SetFocus(modal)
}
