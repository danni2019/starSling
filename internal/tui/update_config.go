//go:build ignore
// +build ignore

package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
)

var configMenuItems = []string{
	"Select config",
	"Edit config",
	"Delete config",
	"Back",
}

var configSaveItems = []string{
	"Save as default",
	"Save as new",
	"Cancel",
}

var configDeleteItems = []string{
	"Confirm delete",
	"Cancel",
}

func (m Model) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
		return m.handleConfigEsc(), nil
	}

	switch m.configState {
	case configMenu:
		return m.updateConfigMenu(msg)
	case configSelect:
		return m.updateConfigSelect(msg)
	case configEditPick:
		return m.updateConfigEditPick(msg)
	case configEditForm:
		return m.updateConfigEditForm(msg)
	case configSaveChoice:
		return m.updateConfigSaveChoice(msg)
	case configSaveName:
		return m.updateConfigSaveName(msg)
	case configDeletePick:
		return m.updateConfigDeletePick(msg)
	case configDeleteConfirm:
		return m.updateConfigDeleteConfirm(msg)
	default:
		return m, nil
	}
}

func (m Model) updateConfigMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.configIndex > 0 {
				m.configIndex--
			}
		case "down", "j":
			if m.configIndex < len(configMenuItems)-1 {
				m.configIndex++
			}
		case "enter":
			switch m.configIndex {
			case 0:
				m.configState = configSelect
				m.configIndex = 0
			case 1:
				m.configState = configEditPick
				m.configIndex = 0
			case 2:
				m.configState = configDeletePick
				m.configIndex = 0
			case 3:
				m.screen = screenMain
			}
		}
	}
	return m, nil
}

func (m Model) updateConfigSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.configIndex > 0 {
				m.configIndex--
			}
		case "down", "j":
			if m.configIndex < len(m.configItems)-1 {
				m.configIndex++
			}
		case "enter":
			if len(m.configItems) == 0 {
				m.configError = "no configs available"
				m.configState = configMenu
				return m, nil
			}
			name := m.configItems[m.configIndex]
			if err := configstore.SetDefault(name); err != nil {
				m.configError = err.Error()
			} else {
				m.configError = ""
			}
			m.configState = configMenu
		}
	}
	return m, nil
}

func (m Model) updateConfigEditPick(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.configIndex > 0 {
				m.configIndex--
			}
		case "down", "j":
			if m.configIndex < len(m.configItems)-1 {
				m.configIndex++
			}
		case "enter":
			if len(m.configItems) == 0 {
				m.configError = "no configs available"
				m.configState = configMenu
				return m, nil
			}
			name := m.configItems[m.configIndex]
			cfg, form, errMsg := loadConfigForEditWithCfg(name)
			if errMsg != "" {
				m.configError = errMsg
				m.configState = configMenu
				return m, nil
			}
			m.configEditingName = name
			m.configEditingCfg = cfg
			m.configForm = form
			m.configFormIndex = 0
			m.configState = configEditForm
		}
	}
	return m, nil
}

func (m Model) updateConfigEditForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.configForm) == 0 {
		m.configState = configMenu
		return m, nil
	}

	var cmd tea.Cmd
	m.configForm[m.configFormIndex], cmd = m.configForm[m.configFormIndex].Update(msg)

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "tab":
			m.configForm[m.configFormIndex].Blur()
			if m.configFormIndex < len(m.configForm)-1 {
				m.configFormIndex++
				m.configForm[m.configFormIndex].Focus()
			} else {
				m.configState = configSaveChoice
			}
		case "shift+tab":
			m.configForm[m.configFormIndex].Blur()
			if m.configFormIndex > 0 {
				m.configFormIndex--
				m.configForm[m.configFormIndex].Focus()
			}
		}
	}

	return m, cmd
}

func (m Model) updateConfigSaveChoice(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.configSaveIndex > 0 {
				m.configSaveIndex--
			}
		case "down", "j":
			if m.configSaveIndex < len(configSaveItems)-1 {
				m.configSaveIndex++
			}
		case "enter":
			switch m.configSaveIndex {
			case 0:
				return m.saveConfig(m.configEditingName, true)
			case 1:
				m.configNameInput = newNameInput("")
				m.configState = configSaveName
			case 2:
				m.configState = configMenu
			}
		}
	}
	return m, nil
}

func (m Model) updateConfigSaveName(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.configNameInput, cmd = m.configNameInput.Update(msg)
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			name := m.configNameInput.Value()
			return m.saveConfig(name, false)
		}
	}
	return m, cmd
}

func (m Model) updateConfigDeletePick(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.configIndex > 0 {
				m.configIndex--
			}
		case "down", "j":
			if m.configIndex < len(m.configItems)-1 {
				m.configIndex++
			}
		case "enter":
			if len(m.configItems) <= 1 {
				m.configError = "cannot delete the last config"
				m.configState = configMenu
				return m, nil
			}
			m.configDeleteName = m.configItems[m.configIndex]
			m.configDeleteChoice = 0
			m.configState = configDeleteConfirm
		}
	}
	return m, nil
}

func (m Model) updateConfigDeleteConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.configDeleteChoice = 0
		case "down", "j":
			m.configDeleteChoice = 1
		case "enter":
			if m.configDeleteChoice == 0 {
				if err := configstore.Delete(m.configDeleteName); err != nil {
					m.configError = err.Error()
				} else {
					m.configError = ""
					m.configItems, _ = configstore.List()
				}
			}
			m.configState = configMenu
		}
	}
	return m, nil
}

func (m Model) handleConfigEsc() Model {
	switch m.configState {
	case configMenu:
		m.screen = screenMain
	case configSelect, configEditPick, configDeletePick:
		m.configState = configMenu
	case configEditForm, configSaveChoice:
		m.configState = configMenu
	case configSaveName:
		m.configState = configSaveChoice
	case configDeleteConfirm:
		m.configState = configMenu
	}
	return m
}

func (m Model) saveConfig(name string, setDefault bool) (tea.Model, tea.Cmd) {
	cfg, err := m.configFromForm()
	if err != nil {
		m.configError = err.Error()
		return m, nil
	}
	normalized, err := configstore.NormalizeName(name)
	if err != nil {
		m.configError = err.Error()
		return m, nil
	}
	if !setDefault {
		exists, err := configstore.Exists(normalized)
		if err != nil {
			m.configError = err.Error()
			return m, nil
		}
		if exists {
			m.configError = "config already exists"
			m.configState = configSaveName
			return m, nil
		}
	}
	if err := configstore.Save(normalized, cfg); err != nil {
		m.configError = err.Error()
		return m, nil
	}
	if setDefault {
		if err := configstore.SetDefault(normalized); err != nil {
			m.configError = err.Error()
			return m, nil
		}
	}
	m.configError = ""
	m.configItems, _ = configstore.List()
	m.configState = configMenu
	return m, nil
}

func (m Model) configFromForm() (config.Config, error) {
	cfg := m.configEditingCfg
	if len(m.configForm) < 6 {
		return config.Config{}, fmt.Errorf("config form incomplete")
	}
	cfg.LiveMD.API = m.configForm[0].Value()
	cfg.LiveMD.Protocol = m.configForm[1].Value()
	cfg.LiveMD.Host = m.configForm[2].Value()
	portValue := m.configForm[3].Value()
	if portValue != "" {
		port, err := strconv.Atoi(portValue)
		if err != nil {
			return config.Config{}, fmt.Errorf("invalid port")
		}
		cfg.LiveMD.Port = port
	} else {
		cfg.LiveMD.Port = 0
	}
	cfg.LiveMD.Username = m.configForm[4].Value()
	cfg.LiveMD.Password = m.configForm[5].Value()
	cfg.Normalize()
	if err := cfg.ValidateLiveMD(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func newNameInput(value string) textinput.Model {
	input := textinput.New()
	input.Prompt = "Config name: "
	input.SetValue(value)
	input.Focus()
	return input
}
