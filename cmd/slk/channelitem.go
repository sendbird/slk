package main

import (
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/slackfmt"
	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/newconvopicker"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/slack-go/slack"
)

// buildChannelItem converts a Slack conversation into the sidebar
// ChannelItem + finder Item shape used everywhere in slk. Pure function:
// reads from wctx for name/presence resolution, returns the constructed
// sidebar item plus a parallel finder entry. The caller decides whether
// to append, upsert, or persist.
//
// Extracted from the workspace-bootstrap loop in initWorkspace so that
// mid-session conversation events (mpim_open / im_created / group_joined /
// channel_joined) can produce identical items.
//
// The returned finder Item uses the same display name as the sidebar item
// (e.g. "alice" for a DM, the formatted participant list for a group DM)
// because that's what the bootstrap loop has always passed to the finder
// and what the finder's filter/render code expects.
func buildChannelItem(ch slack.Channel, wctx *WorkspaceContext, cfg config.Config, teamID string) (sidebar.ChannelItem, channelfinder.Item) {
	chType := "channel"
	if ch.IsIM {
		// Slack returns the same is_im=true for human DMs and app DMs;
		// the only differentiator is the peer user's IsBot/IsAppUser
		// flag, which we look up via the cache-seeded BotUserIDs set.
		// Unknown peers default to "dm" and are reclassified later by
		// the resolveUser path.
		if wctx.BotUserIDs[ch.User] {
			chType = "app"
		} else {
			chType = "dm"
		}
	} else if ch.IsMpIM {
		chType = "group_dm"
	} else if ch.IsPrivate {
		chType = "private"
	}

	displayName := ch.Name
	if ch.IsIM {
		if resolved, ok := wctx.UserNames[ch.User]; ok {
			displayName = resolved
		} else {
			displayName = ch.User
		}
	} else if ch.IsMpIM {
		displayName = slackfmt.FormatMPDMName(ch.Name, func(h string) string {
			return wctx.UserNamesByHandle[h]
		})
	}

	section := ""
	if wctx.SectionStore != nil && wctx.SectionStore.Ready() {
		if id, ok := wctx.SectionStore.SectionForChannel(ch.ID); ok {
			section = id
		}
	}
	var sectionOrder int
	if section == "" {
		section = cfg.MatchSection(teamID, ch.Name)
		if section != "" {
			sectionOrder = cfg.SectionOrder(teamID, section)
		}
	}

	muted := false
	if wctx.MuteStore != nil {
		muted = wctx.MuteStore.IsMuted(ch.ID)
	}

	item := sidebar.ChannelItem{
		ID:           ch.ID,
		Name:         displayName,
		Type:         chType,
		Section:      section,
		SectionOrder: sectionOrder,
		IsMuted:      muted,
	}
	if ch.IsIM {
		item.DMUserID = ch.User
	}

	finderItem := channelfinder.Item{
		ID:       ch.ID,
		Name:     displayName,
		Type:     chType,
		Presence: item.Presence,
		Joined:   true,
	}
	return item, finderItem
}

// upsertChannelInDB writes the channel to the SQLite cache. Separated from
// buildChannelItem so the latter stays a pure function.
func upsertChannelInDB(db *cache.DB, ch slack.Channel, chType string, teamID string) {
	db.UpsertChannel(cache.Channel{
		ID:          ch.ID,
		WorkspaceID: teamID,
		Name:        ch.Name,
		Type:        chType,
		Topic:       ch.Topic.Value,
		IsMember:    ch.IsMember,
	})
}

// buildPickerUsers projects the workspace's cached users into the
// shape consumed by the new-conversation picker. Excludes self
// (currentUserID) and bots; carries IsExternal, presence, and the
// short username from cache.User.Name. Callers that know the
// userID→DM channel mapping (built from sidebar items or the
// slack.Channel list) should pass it via applyDMChannelIDs to
// populate DMChannelID, which lets the picker short-circuit Enter
// for known users to a channel switch.
func buildPickerUsers(users []cache.User, currentUserID string) []newconvopicker.Item {
	out := make([]newconvopicker.Item, 0, len(users))
	for _, u := range users {
		if u.ID == currentUserID || u.IsBot {
			continue
		}
		name := u.DisplayName
		if name == "" {
			name = u.Name
		}
		if name == "" {
			name = u.ID
		}
		out = append(out, newconvopicker.Item{
			ID:         u.ID,
			Kind:       newconvopicker.KindUser,
			Name:       name,
			Username:   u.Name,
			Presence:   u.Presence,
			IsExternal: u.IsExternal,
		})
	}
	return out
}

// applyDMChannelIDs fills out the DMChannelID field on picker user
// items using a userID → channelID map. main.go has access to the
// raw slack.Channel slice (with .User on IM rows), which is what
// produces this mapping.
func applyDMChannelIDs(items []newconvopicker.Item, dmByUser map[string]string) []newconvopicker.Item {
	if len(dmByUser) == 0 {
		return items
	}
	for i := range items {
		if items[i].Kind != newconvopicker.KindUser {
			continue
		}
		if ch, ok := dmByUser[items[i].ID]; ok {
			items[i].DMChannelID = ch
		}
	}
	return items
}

// dmChannelByUser projects sidebar items into a userID → DM channelID
// map. Only "dm" rows with a non-empty DMUserID contribute. Used by
// the new-conversation picker so it can short-circuit Enter on a
// known user to a direct channel switch rather than re-calling
// conversations.open.
func dmChannelByUser(items []sidebar.ChannelItem) map[string]string {
	out := make(map[string]string, len(items))
	for _, it := range items {
		if it.Type == "dm" && it.DMUserID != "" {
			out[it.DMUserID] = it.ID
		}
	}
	return out
}

// upsertWctxChannel records a freshly-opened DM/mpim onto the
// WorkspaceContext so the channel survives a workspace switch and is
// included on the next bootstrap. Idempotent: replaces by channel ID
// when present in either slice, appends otherwise. For 1:1 DMs it
// also patches the matching PickerUsers row to wire DMChannelID,
// preventing the picker from re-opening the same conversation.
func upsertWctxChannel(wctx *WorkspaceContext, sidebarItem sidebar.ChannelItem, finderItem channelfinder.Item, userIDs []string) {
	if wctx == nil {
		return
	}
	replaced := false
	for i, it := range wctx.Channels {
		if it.ID == sidebarItem.ID {
			wctx.Channels[i] = sidebarItem
			replaced = true
			break
		}
	}
	if !replaced {
		wctx.Channels = append(wctx.Channels, sidebarItem)
	}
	replaced = false
	for i, it := range wctx.FinderItems {
		if it.ID == finderItem.ID {
			wctx.FinderItems[i] = finderItem
			replaced = true
			break
		}
	}
	if !replaced {
		wctx.FinderItems = append(wctx.FinderItems, finderItem)
	}
	// Wire DMChannelID on the matching PickerUsers row for 1:1 DMs so
	// the picker can short-circuit future Enters on this user.
	if sidebarItem.Type == "dm" && sidebarItem.DMUserID != "" {
		for i := range wctx.PickerUsers {
			if wctx.PickerUsers[i].ID == sidebarItem.DMUserID {
				wctx.PickerUsers[i].DMChannelID = sidebarItem.ID
				break
			}
		}
	}
}
