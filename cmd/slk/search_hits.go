package main

import (
	"fmt"
	"strings"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui/globalsearch"
)

// messageHitsToItems converts the slack client's normalized message
// search hits into globalsearch Items. `userNames` is the workspace's
// id -> display-name cache so we can resolve the author when the hit
// only carries a user id.
func messageHitsToItems(hits []slackclient.MessageSearchHit, userNames map[string]string) []globalsearch.Item {
	out := make([]globalsearch.Item, 0, len(hits))
	for _, h := range hits {
		name := h.Username
		if name == "" {
			if resolved, ok := userNames[h.UserID]; ok {
				name = resolved
			}
		}
		if name == "" {
			name = h.UserID
		}
		channelLabel := h.ChannelName
		if channelLabel != "" {
			channelLabel = "#" + channelLabel
		}
		// Squash newlines and condense the preview so the row stays
		// on one terminal line.
		preview := strings.ReplaceAll(h.Text, "\n", " ")
		preview = strings.TrimSpace(preview)
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		label := fmt.Sprintf("%s — %s", name, preview)
		out = append(out, globalsearch.Item{
			ID:          h.TS,
			Name:        label,
			Subtitle:    channelLabel,
			Type:        "message",
			ChannelID:   h.ChannelID,
			ChannelName: h.ChannelName,
			MessageTS:   h.TS,
			Permalink:   h.Permalink,
		})
	}
	return out
}

func fileHitsToItems(hits []slackclient.FileSearchHit, userNames map[string]string) []globalsearch.Item {
	out := make([]globalsearch.Item, 0, len(hits))
	for _, h := range hits {
		title := h.Title
		if title == "" {
			title = h.Name
		}
		if title == "" {
			title = h.ID
		}
		uploader := ""
		if resolved, ok := userNames[h.UserID]; ok {
			uploader = resolved
		} else if h.UserID != "" {
			uploader = h.UserID
		}
		subtitle := h.Mimetype
		if uploader != "" {
			if subtitle != "" {
				subtitle = uploader + " · " + subtitle
			} else {
				subtitle = uploader
			}
		}
		out = append(out, globalsearch.Item{
			ID:        h.ID,
			Name:      title,
			Subtitle:  subtitle,
			Type:      "file",
			Permalink: h.Permalink,
		})
	}
	return out
}
