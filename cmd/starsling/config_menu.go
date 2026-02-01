package main

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
)

const (
	configActionUse    = "use"
	configActionEdit   = "edit"
	configActionDelete = "delete"
	configActionBack   = "back"

	configSaveDefault = "default"
	configSaveNew     = "new"
	configSaveCancel  = "cancel"
)

func runConfigMenu(logger *slog.Logger) error {
	if _, err := configstore.Ensure(); err != nil {
		return err
	}

	for {
		action, err := promptConfigMenu()
		if err != nil {
			return err
		}

		switch action {
		case configActionBack:
			return nil
		case configActionUse:
			if err := runConfigSelect(); err != nil {
				if errors.Is(err, errForceExit) {
					return errForceExit
				}
				if errors.Is(err, errUserAborted) {
					continue
				}
				logger.Error("config selection failed", "error", err)
			}
		case configActionEdit:
			if err := runConfigEdit(logger); err != nil {
				if errors.Is(err, errForceExit) {
					return errForceExit
				}
				if errors.Is(err, errUserAborted) {
					continue
				}
				logger.Error("config edit failed", "error", err)
			}
		case configActionDelete:
			if err := runConfigDelete(logger); err != nil {
				if errors.Is(err, errForceExit) {
					return errForceExit
				}
				if errors.Is(err, errUserAborted) {
					continue
				}
				logger.Error("config delete failed", "error", err)
			}
		default:
			return fmt.Errorf("unknown config action: %s", action)
		}
	}
}

func promptConfigMenu() (string, error) {
	action := configActionUse
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Config").
				Options(
					huh.NewOption("Use existing config", configActionUse),
					huh.NewOption("Create or update config", configActionEdit),
					huh.NewOption("Delete config", configActionDelete),
					huh.NewOption("Back", configActionBack),
				).
				Value(&action),
		),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return action, nil
}

func runConfigSelect() error {
	names, err := configstore.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no configs available")
	}
	selected, err := promptConfigSelect("Select config", names)
	if err != nil {
		return err
	}
	if err := configstore.SetDefault(selected); err != nil {
		return err
	}
	return nil
}

func runConfigEdit(logger *slog.Logger) error {
	defaultName, cfg, err := configstore.LoadDefault()
	if err != nil {
		return err
	}

	for {
		liveCfg, err := promptLiveConnection(cfg.LiveMD)
		if err != nil {
			return err
		}
		cfg.LiveMD = liveCfg
		if err := cfg.ValidateLiveMD(); err != nil {
			logger.Error("live-md config error", "error", err)
			continue
		}

		saveAction, err := promptConfigSaveAction()
		if err != nil {
			return err
		}
		switch saveAction {
		case configSaveCancel:
			return nil
		case configSaveDefault:
			if err := configstore.Save(defaultName, cfg); err != nil {
				return err
			}
			return configstore.SetDefault(defaultName)
		case configSaveNew:
			name, err := promptConfigName()
			if err != nil {
				return err
			}
			return saveNewConfig(name, cfg)
		default:
			return fmt.Errorf("unknown save action: %s", saveAction)
		}
	}
}

func saveNewConfig(name string, cfg config.Config) error {
	normalized, err := configstore.NormalizeName(name)
	if err != nil {
		return err
	}
	exists, err := configstore.Exists(normalized)
	if err != nil {
		return err
	}
	if exists {
		overwrite, err := promptConfirm("Config exists", "Overwrite existing config?")
		if err != nil {
			return err
		}
		if !overwrite {
			return errUserAborted
		}
	}
	if err := configstore.Save(normalized, cfg); err != nil {
		return err
	}
	return nil
}

func runConfigDelete(logger *slog.Logger) error {
	names, err := configstore.List()
	if err != nil {
		return err
	}
	if len(names) <= 1 {
		logger.Warn("cannot delete the last remaining config")
		return nil
	}
	selected, err := promptConfigSelect("Delete config", names)
	if err != nil {
		return err
	}
	confirm, err := promptConfirm("Delete config", fmt.Sprintf("Delete %q? This cannot be undone.", selected))
	if err != nil {
		return err
	}
	if !confirm {
		return nil
	}
	return configstore.Delete(selected)
}

func promptConfigSelect(title string, names []string) (string, error) {
	if len(names) == 0 {
		return "", fmt.Errorf("no configs available")
	}
	selected := names[0]
	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		options = append(options, huh.NewOption(name, name))
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(&selected),
		),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return selected, nil
}

func promptConfigSaveAction() (string, error) {
	action := configSaveDefault
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Save config").
				Options(
					huh.NewOption("Save as default", configSaveDefault),
					huh.NewOption("Save as new config", configSaveNew),
					huh.NewOption("Cancel", configSaveCancel),
				).
				Value(&action),
		),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return action, nil
}

func promptConfigName() (string, error) {
	name := ""
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Config name").
				Placeholder("my-config").
				Validate(validateConfigName).
				Value(&name),
		),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return strings.TrimSpace(name), nil
}

func validateConfigName(value string) error {
	_, err := configstore.NormalizeName(value)
	return err
}
