//go:build ignore
// +build ignore

package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
	"github.com/danni2019/starSling/internal/runtime"
)

type screen int

const (
	screenMain screen = iota
	screenLive
	screenSetup
	screenConfig
	screenDrilldown
)

type configState int

const (
	configMenu configState = iota
	configSelect
	configEditPick
	configEditForm
	configSaveChoice
	configSaveName
	configDeletePick
	configDeleteConfirm
)

type focusArea int

const (
	focusMarketTable focusArea = iota
	focusCurveChart
	focusOptionsChart
	focusUnusualTrades
	focusLogPanel
)

type tickMsg time.Time
type setupResultMsg struct {
	output string
	err    error
}

type Model struct {
	screen      screen
	focus       focusArea
	menuIndex   int
	marketIndex int
	width       int
	height      int
	data        MockData
	mainFrame   int

	setupIndex   int
	setupRunning bool
	setupOutput  []string
	setupError   string

	configState        configState
	configIndex        int
	configItems        []string
	configError        string
	configForm         []textinput.Model
	configFormIndex    int
	configEditingName  string
	configEditingCfg   config.Config
	configSaveIndex    int
	configNameInput    textinput.Model
	configDeleteName   string
	configDeleteChoice int
}

func NewModel() Model {
	items, err := configstore.List()
	configErr := ""
	if err != nil {
		configErr = err.Error()
	}
	if len(items) == 0 {
		items = []string{}
	}
	return Model{
		screen:      screenMain,
		focus:       focusMarketTable,
		data:        mockData(),
		configItems: items,
		configError: configErr,
	}
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			return m.handleEsc(), nil
		}
	case tickMsg:
		m.data = m.data.Tick()
		m.mainFrame = (m.mainFrame + 1) % 4
		return m, tickCmd()
	case setupResultMsg:
		m.setupRunning = false
		m.setupOutput = splitLines(msg.output)
		if msg.err != nil {
			m.setupError = msg.err.Error()
		} else {
			m.setupError = ""
		}
		return m, nil
	}

	switch m.screen {
	case screenMain:
		return m.updateMain(msg)
	case screenLive:
		return m.updateLive(msg)
	case screenSetup:
		return m.updateSetup(msg)
	case screenConfig:
		return m.updateConfig(msg)
	case screenDrilldown:
		return m.updateDrilldown(msg)
	default:
		return m, nil
	}
}

func (m Model) View() string {
	switch m.screen {
	case screenMain:
		return m.viewMain()
	case screenLive:
		return m.viewLive()
	case screenSetup:
		return m.viewSetup()
	case screenConfig:
		return m.viewConfig()
	case screenDrilldown:
		return m.viewPlaceholder("Drilldown")
	default:
		return ""
	}
}

func (m Model) updateMain(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.menuIndex > 0 {
				m.menuIndex--
			}
		case "down", "j":
			if m.menuIndex < len(mainMenuItems)-1 {
				m.menuIndex++
			}
		case "enter":
			switch mainMenuItems[m.menuIndex].id {
			case menuLive:
				m.screen = screenLive
			case menuSetup:
				m.screen = screenSetup
				m.setupIndex = 0
			case menuConfig:
				m.screen = screenConfig
				m.configState = configMenu
				m.configIndex = 0
				m.configItems, m.configError = ensureConfigList()
			case menuQuit:
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) updateLive(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab", "right":
			m.focus = nextFocus(m.focus)
		case "shift+tab", "left":
			m.focus = prevFocus(m.focus)
		case "up", "k":
			if m.focus == focusMarketTable && m.marketIndex > 0 {
				m.marketIndex--
			}
		case "down", "j":
			if m.focus == focusMarketTable && m.marketIndex < len(m.data.MarketRows)-1 {
				m.marketIndex++
			}
		case "enter":
			m.screen = screenDrilldown
		}
	}
	return m, nil
}

func (m Model) updateDrilldown(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			if m.screen == screenDrilldown {
				m.screen = screenLive
			}
		}
	}
	return m, nil
}

func (m Model) handleEsc() Model {
	switch m.screen {
	case screenLive, screenSetup, screenConfig:
		m.screen = screenMain
	case screenDrilldown:
		m.screen = screenLive
	}
	return m
}

func nextFocus(current focusArea) focusArea {
	return focusArea((int(current) + 1) % int(focusLogPanel+1))
}

func prevFocus(current focusArea) focusArea {
	if current == 0 {
		return focusLogPanel
	}
	return focusArea(int(current) - 1)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func setupBootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		output, err := runtime.RunBootstrap()
		return setupResultMsg{output: output, err: err}
	}
}

func ensureConfigList() ([]string, string) {
	if _, err := configstore.Ensure(); err != nil {
		return nil, err.Error()
	}
	items, err := configstore.List()
	if err != nil {
		return nil, err.Error()
	}
	return items, ""
}

func loadConfigForEdit(name string) ([]textinput.Model, string) {
	cfg, err := configstore.Load(name)
	if err != nil {
		return nil, err.Error()
	}
	return buildConfigForm(cfg), ""
}

func loadConfigForEditWithCfg(name string) (config.Config, []textinput.Model, string) {
	cfg, err := configstore.Load(name)
	if err != nil {
		return config.Config{}, nil, err.Error()
	}
	return cfg, buildConfigForm(cfg), ""
}

func buildConfigForm(cfg config.Config) []textinput.Model {
	fields := []struct {
		label string
		value string
	}{
		{label: "API", value: cfg.LiveMD.API},
		{label: "Protocol", value: cfg.LiveMD.Protocol},
		{label: "Host", value: cfg.LiveMD.Host},
		{label: "Port", value: formatInt(cfg.LiveMD.Port)},
		{label: "Username", value: cfg.LiveMD.Username},
		{label: "Password", value: cfg.LiveMD.Password},
	}

	inputs := make([]textinput.Model, 0, len(fields))
	for _, field := range fields {
		input := textinput.New()
		input.Prompt = field.label + ": "
		input.SetValue(field.value)
		if field.label == "Password" {
			input.EchoMode = textinput.EchoPassword
			input.EchoCharacter = '*'
		}
		inputs = append(inputs, input)
	}
	if len(inputs) > 0 {
		inputs[0].Focus()
	}
	return inputs
}

func formatInt(value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}
