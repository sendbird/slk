package main

import (
	"testing"

	"github.com/gammons/slk/internal/ui"
)

func TestChannelSearchPrefix(t *testing.T) {
	tests := []struct {
		name  string
		scope ui.ChannelSearchScope
		want  string
	}{
		{
			name:  "public channel",
			scope: ui.ChannelSearchScope{Name: "eng-deploy", Type: "channel"},
			want:  "in:eng-deploy",
		},
		{
			name:  "private channel",
			scope: ui.ChannelSearchScope{Name: "eng-secret", Type: "private"},
			want:  "in:eng-secret",
		},
		{
			name:  "dm",
			scope: ui.ChannelSearchScope{Name: "jinku", Type: "dm", DMUserID: "U123"},
			want:  "with:<@U123>",
		},
		{
			name:  "app dm",
			scope: ui.ChannelSearchScope{Name: "github", Type: "app", DMUserID: "UAPP"},
			want:  "with:<@UAPP>",
		},
		{
			name:  "group dm unsupported",
			scope: ui.ChannelSearchScope{Name: "a,b", Type: "group_dm"},
			want:  "",
		},
		{
			name:  "dm missing user id",
			scope: ui.ChannelSearchScope{Name: "jinku", Type: "dm"},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelSearchPrefix(tt.scope); got != tt.want {
				t.Fatalf("channelSearchPrefix(%+v) = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}
