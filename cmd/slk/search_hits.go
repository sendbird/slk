package main

import (
	"fmt"
	"strings"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/globalsearch"
)

// channelSearchPrefix turns a Ctrl+F scope into the Slack search
// query prefix that restricts results to the active channel or DM.
//
// Mappings:
//   - channel / private  → "in:<name>"
//   - dm / app           → "with:<@U…>" when DMUserID is set; otherwise empty
//   - group_dm           → empty (the App's open path already filters this out)
//
// Slack's search modifier accepts `in:<name>` without the leading `#`
// — the web client autocompletes the hash for display, but the API
// parses the modifier on the bare name. Adding the `#` previously
// caused the API to fall back to an unfiltered search on some
// workspaces.
//
// Returns an empty string if no prefix can be derived, in which case
// the caller should run an unscoped search rather than block on a
// scope mismatch.
func channelSearchPrefix(scope ui.ChannelSearchScope) string {
	switch scope.Type {
	case "channel", "private":
		if scope.Name != "" {
			return "in:" + scope.Name
		}
	case "dm", "app":
		if scope.DMUserID != "" {
			return "with:<@" + scope.DMUserID + ">"
		}
	}
	return ""
}

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
