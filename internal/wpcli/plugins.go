package wpcli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// GetPlugins returns a list of installed plugins on the target site.
func GetPlugins(site models.CliSite, skipPlugins bool) ([]models.WPPlugin, error) {
	res, err := RunWP(CliOptions{SSH: site.SSH, Path: site.Path, IncludePlugins: !skipPlugins}, "plugin", "list", "--format=json")
	if err != nil {
		return nil, err
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
	args := []string{"plugin", "install", plugin}

	// If the plugin is a ZIP file (local or URL), add --force to allow updating.
	lowerPlugin := strings.ToLower(plugin)
	if strings.HasSuffix(lowerPlugin, ".zip") || strings.Contains(lowerPlugin, ".zip?") {
		args = append(args, "--force")
	}

	if activate {
		args = append(args, "--activate")
	}
	res, err := RunWP(CliOptions{SSH: ssh, Path: path, IncludePlugins: true}, args...)
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

// UpdatePlugin updates one or more plugins.
func UpdatePlugin(ssh, path string, plugins []string) ([]UpdateResult, error) {
	if len(plugins) == 0 {
		return nil, nil
	}

	args := []string{"plugin", "update"}
	args = append(args, plugins...)
	args = append(args, "--format=json")

	res, err := RunWP(CliOptions{SSH: ssh, Path: path, IncludePlugins: true}, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w (stderr: %s)", err, res.Error)
	}
	var updates []UpdateResult
	if err := json.Unmarshal([]byte(res.Output), &updates); err != nil {
		return nil, fmt.Errorf("failed to parse update result: %w", err)
	}
	for _, update := range updates {
		if update.Status == "Updated" {
			verb.Printf(verb.Normal, "Updated %s from %s to %s\n", update.Name, update.OldVersion, update.NewVersion)
		} else {
			verb.Printf(verb.Normal, "Failed to update %s: %s\n", update.Name, update.Status)
		}
	}
	return updates, nil
}

// RemovePlugin uninstalls and deactivates a plugin.
func RemovePlugin(ssh, path, plugin string) (bool, error) {
	res, err := RunWP(CliOptions{SSH: ssh, Path: path, IncludePlugins: true}, "plugin", "uninstall", plugin, "--deactivate")
	if err != nil {
		return false, fmt.Errorf("failed to remove plugin: %w (stderr: %s)", err, res.Error)
	}
	return strings.Contains(res.Output, "Success: Uninstalled"), nil
}

func GetPluginInfo(ssh, path, plugin string) (*models.PluginInfo, error) {
	res, err := RunWP(CliOptions{SSH: ssh, Path: path, IncludePlugins: true}, "plugin", "get", plugin, "--format=json")
	if err != nil {
		if strings.Contains(res.Error, "plugin could not be found.") {
			return nil, fmt.Errorf("plugin %s not found", plugin)
		}
		return nil, err
	}

	type rawInfo struct {
		Slug        string `json:"name"`
		Name        string `json:"title"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Status      string `json:"status"`
	}

	var info rawInfo
	if err := json.Unmarshal([]byte(res.Output), &info); err != nil {
		return nil, fmt.Errorf("failed to parse plugin info JSON: %w", err)
	}

	return &models.PluginInfo{
		Name:    info.Name,
		Slug:    info.Slug,
		Version: info.Version,
		Author:  info.Author,
	}, nil
}
