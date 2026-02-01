package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/metadata"
)

const (
	modeLive  = "live"
	modeSetup = "setup"
	modeConfig = "config"
	modeQuit  = "quit"
)

var (
	errUserAborted = errors.New("user aborted")
	errForceExit   = errors.New("force exit")
)

func promptMode() (string, error) {
	mode := modeLive
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Start mode").
				Options(
					huh.NewOption("Live market data", modeLive),
					huh.NewOption("Setup Python runtime", modeSetup),
					huh.NewOption("Config", modeConfig),
					huh.NewOption("Quit", modeQuit),
				).
				Value(&mode),
		),
	)

	if err := runForm(form); err != nil {
		return "", err
	}
	return mode, nil
}

func promptLiveConnection(liveCfg config.LiveMDConfig) (config.LiveMDConfig, error) {
	api := liveCfg.API
	protocol := liveCfg.Protocol
	host := liveCfg.Host
	port := liveCfg.Port
	username := liveCfg.Username
	password := liveCfg.Password

	portStr := ""
	if port > 0 {
		portStr = strconv.Itoa(port)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Market data API").
				Options(
					huh.NewOption("CTP", "ctp"),
				).
				Value(&api),
			huh.NewSelect[string]().
				Title("Protocol").
				Options(
					huh.NewOption("TCP", "tcp"),
				).
				Value(&protocol),
			huh.NewInput().
				Title("Host").
				Validate(nonEmpty("host")).
				Value(&host),
			huh.NewInput().
				Title("Port").
				Validate(validPort).
				Value(&portStr),
			huh.NewInput().
				Title("Username (optional)").
				Value(&username),
			huh.NewInput().
				Title("Password (optional)").
				EchoMode(huh.EchoModePassword).
				Value(&password),
		),
	)

	if err := runForm(form); err != nil {
		return config.LiveMDConfig{}, err
	}

	parsedPort, err := strconv.Atoi(portStr)
	if err != nil {
		return config.LiveMDConfig{}, fmt.Errorf("invalid port")
	}

	liveCfg = config.LiveMDConfig{
		API:         strings.ToLower(strings.TrimSpace(api)),
		Protocol:    strings.ToLower(strings.TrimSpace(protocol)),
		Host:        strings.TrimSpace(host),
		Port:        parsedPort,
		Username:    strings.TrimSpace(username),
		Password:    password,
		Instruments: nil,
	}

	return liveCfg, nil
}

func promptLiveInstruments(configName string) ([]string, error) {
	value := ""
	title := "Instruments"
	if strings.TrimSpace(configName) != "" {
		title = fmt.Sprintf("Instruments (config: %s)", configName)
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
				Description("Leave blank to subscribe to all available contracts.").
				Value(&value),
		),
	)
	if err := runForm(form); err != nil {
		return nil, err
	}
	return splitList(value), nil
}

func runForm(form *huh.Form) error {
	form.WithKeyMap(formKeyMap())
	form.WithInput(ctrlCFile{File: os.Stdin})
	if err := form.Run(); err != nil {
		if errors.Is(err, errForceExit) {
			return errForceExit
		}
		if errors.Is(err, huh.ErrUserAborted) {
			return errUserAborted
		}
		return err
	}
	return nil
}

type ctrlCFile struct {
	*os.File
}

func (c ctrlCFile) Read(p []byte) (int, error) {
	n, err := c.File.Read(p)
	if n > 0 {
		for i := 0; i < n; i++ {
			if p[i] == 0x03 {
				return 0, errForceExit
			}
		}
	}
	return n, err
}

func formKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Quit = key.NewBinding(key.WithKeys("esc"))
	return keymap
}

func nonEmpty(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

func validPort(value string) error {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || parsed > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func promptConfirm(title, description string) (bool, error) {
	confirm := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative("Confirm").
				Negative("Cancel").
				Value(&confirm),
		),
	)
	if err := runForm(form); err != nil {
		return false, err
	}
	return confirm, nil
}

func splitList(value string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func promptMetadataWarning(warnings []metadata.Warning) error {
	if len(warnings) == 0 {
		return nil
	}

	lines := make([]string, 0, len(warnings))
	for _, warn := range warnings {
		age := "unknown"
		if !warn.LastUpdated.IsZero() {
			age = fmt.Sprintf("%s old", warn.Age.Round(time.Minute))
		}
		detail := warn.Name + ": " + age
		if warn.LastError != "" {
			detail += " (" + warn.LastError + ")"
		}
		lines = append(lines, detail)
	}

	message := strings.Join(lines, "\n")
	proceed := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Metadata stale warning").
				Description(message).
				Affirmative("Proceed").
				Negative("Back").
				Value(&proceed),
		),
	)

	if err := runForm(form); err != nil {
		return err
	}
	if !proceed {
		return errUserAborted
	}
	return nil
}
