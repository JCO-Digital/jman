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

func UpdatePlugin(ssh, path, plugin string) error {
	cmd := fmt.Sprintf("plugin update %s --format=json", plugin)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		return fmt.Errorf("failed to update plugin: %w (stderr: %s)", err, res.Error)
	}
	var updates []UpdateResult
	if err := json.Unmarshal([]byte(res.Output), &updates); err != nil {
		return fmt.Errorf("failed to parse update result: %w", err)
	}
	if len(updates) == 0 {
		return fmt.Errorf("Failed to update '%s'.\n", plugin)
	}
	if updates[0].Status != "Updated" {
		return fmt.Errorf("Failed to update '%s'. Status: %s\n", plugin, updates[0].Status)
	}
	verbosity.Printf(verbosity.Verbose, "Updated to %s\n", updates[0].NewVersion)
	return nil
}
