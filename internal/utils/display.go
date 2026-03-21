package utils

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/verb"
)

func DisplayPluginName(slug string, truncate, color bool) string {
	name := cache.GetPluginName(slug)
	if name != slug {
		name = CleanHTML(name)

		if truncate {
			name = ShowFirstPart(name)
		}

		if color {
			name = verb.Yellow(name)
			slug = verb.Cyan(slug)
		}

		return fmt.Sprintf("%s (%s)", name, slug)
	}
	if color {
		slug = verb.Yellow(slug)
	}

	return slug
}
