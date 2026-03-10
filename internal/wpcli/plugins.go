package wpcli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// GetPlugins returns a list of installed plugins on the target site.
func GetPlugins(site models.CliSite) ([]models.WPPlugin, error) {
	res, err := RunWP(site.SSH, site.Path, "plugin list --format=json", true)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, fmt.Errorf("not a WordPress site")
		}
		return nil, fmt.Errorf("unknown error: %w", err)
	}

	output := res.Output
	idx := strings.Index(output, "[")
	if idx != -1 {
		output = output[idx:]
	} else {
		return nil, fmt.Errorf("no valid JSON array found in output")
	}

	type rawPlugin struct {
		Name          string `json:"name"`
		Status        string `json:"status"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
		AutoUpdate    string `json:"auto_update"`
	}

	var raw []rawPlugin
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse plugins JSON: %w", err)
	}

	var plugins []models.WPPlugin
	for _, rp := range raw {
		plugins = append(plugins, models.WPPlugin{
			SiteID:     site.ID,
			Name:       rp.Name,
			Status:     rp.Status,
			Version:    rp.Version,
			Update:     rp.UpdateVersion,
			AutoUpdate: rp.AutoUpdate == "on",
		})
	}

	return plugins, nil
}

// AddPlugin installs and optionally activates a plugin.
func AddPlugin(ssh, path, plugin string, activate bool) (bool, error) {
	activateFlag := ""
	if activate {
		activateFlag = "--activate"
	}
	cmd := fmt.Sprintf("plugin install %s %s", plugin, activateFlag)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		if strings.Contains(res.Error, "Plugin not found.") {
			return false, fmt.Errorf("plugin not found")
		} else if strings.Contains(res.Error, "Destination folder already exists.") {
			return false, fmt.Errorf("plugin already installed")
		}
		return false, fmt.Errorf("failed to install plugin: %w (stderr: %s)", err, res.Error)
	}
	return strings.Contains(res.Output, "Success:"), nil
}

type UpdateResult struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`
}

func UpdatePlugin(ssh, path, pluginList string) (int, error) {
	cmd := fmt.Sprintf("plugin update %s --format=json", pluginList)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		return 0, fmt.Errorf("failed to update plugin: %w (stderr: %s)", err, res.Error)
	}
	var updates []UpdateResult
	if err := json.Unmarshal([]byte(res.Output), &updates); err != nil {
		return 0, fmt.Errorf("failed to parse update result: %w", err)
	}
	updated := 0
	for _, update := range updates {
		if update.Status == "Updated" {
			updated++
			verbosity.Printf(verbosity.Normal, "Updated %s from %s to %s\n", update.Name, update.OldVersion, update.NewVersion)
		} else {
			verbosity.Printf(verbosity.Normal, "Failed to update %s: %s\n", update.Name, update.Status)
		}
	}
	return updated, nil
}

func RemovePlugin(ssh, path, plugin string) (bool, error) {
	cmd := fmt.Sprintf("plugin uninstall %s --deactivate", plugin)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		return false, fmt.Errorf("failed to remove plugin: %w (stderr: %s)", err, res.Error)
	}
	return strings.Contains(res.Output, "Success: Uninstalled"), nil
}
