package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gotd/cli/internal/output"
)

func assertTextRows(t *testing.T, value any, rows, tabs int, escaped ...string) {
	t.Helper()
	var buf bytes.Buffer
	if err := output.New(output.Text, &buf).Emit(value); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if count := strings.Count(got, "\n"); count != rows {
		t.Errorf("text output has %d rows, want %d: %q", count, rows, got)
	}
	if count := strings.Count(got, "\t"); count != tabs {
		t.Errorf("text output has %d tabs, want %d structural tabs: %q", count, tabs, got)
	}
	for _, want := range escaped {
		if !strings.Contains(got, want) {
			t.Errorf("text output %q missing escaped field %q", got, want)
		}
	}
}

func TestPeerRefTextEscapesRemoteLabelControls(t *testing.T) {
	assertTextRows(t, peerRef{ID: 7, Type: peerChannel, Name: "Group\nspoof\trow"}, 1, 2, `Group\nspoof\trow`)
}

func TestHistoryTextEscapesRemoteMessageControls(t *testing.T) {
	assertTextRows(t, historyResult{Messages: []messageItem{{ID: 7, Text: "hello\nspoof\trow"}}}, 1, 0, `hello\nspoof\trow`)
}

func TestChatFullTextEscapesRemoteAboutAndTitleControls(t *testing.T) {
	assertTextRows(t, chatFullResult{
		Peer:  peerRef{ID: 7, Type: peerChannel, Name: "Group\nspoof\trow"},
		About: "about\nspoof\trow",
	}, 2, 0, `Group\nspoof\trow`, `about\nspoof\trow`)
}

func TestRemoteTitleTextEscapesEmbeddedRows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		field string
	}{
		{name: "topic", value: topicsResult{Topics: []topicItem{{ID: 1, Title: "Topic\nspoof\trow"}}}, field: `Topic\nspoof\trow`},
		{name: "folder", value: foldersResult{Folders: []folderItem{{ID: 1, Title: "Folder\nspoof\trow"}}}, field: `Folder\nspoof\trow`},
		{name: "sticker set", value: stickerSetsResult{Sets: []stickerSetItem{{Title: "Pack\nspoof", ShortName: "short\tname", Count: 1}}}, field: `Pack\nspoof`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertTextRows(t, tc.value, 1, 0, tc.field)
		})
	}
}

func TestOtherRemoteTextFieldsEscapeEmbeddedRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  any
		rows   int
		tabs   int
		fields []string
	}{
		{
			name: "chat preview",
			value: chatList{Chats: []chatItem{{
				Peer: peerRef{Type: peerChat, Name: "Group\nspoof\trow"}, LastMessage: "hello\nspoof\trow",
			}}},
			rows: 1, fields: []string{`Group\nspoof\trow`, `hello\nspoof\trow`},
		},
		{
			name: "draft",
			value: draftsResult{Drafts: []draftItem{{
				Peer: peerRef{Type: peerUser, Name: "Alice\nspoof\trow"}, Message: "draft\nspoof\trow",
			}}},
			rows: 1, fields: []string{`Alice\nspoof\trow`, `draft\nspoof\trow`},
		},
		{
			name: "device metadata",
			value: devicesResult{Current: &deviceInfo{
				DeviceModel: "Model\nspoof\trow", AppName: "App\nspoof", IP: "127.0.0.1\nspoof",
			}},
			rows: 6, fields: []string{`Model\nspoof\trow`, `App\nspoof`, `127.0.0.1\nspoof`},
		},
		{
			name:  "reaction",
			value: reactionsResult{Reactions: []reactionItem{{Reaction: "👍\nspoof\trow", Count: 1}}},
			rows:  1, fields: []string{"👍\\nspoof\\trow"},
		},
		{
			name:  "link",
			value: linkResult{Link: "https://t.me/example\nspoof\trow"},
			rows:  1, fields: []string{`https://t.me/example\nspoof\trow`},
		},
		{
			name:  "download path",
			value: downloadResult{Path: "file\nspoof\trow"},
			rows:  1, fields: []string{`file\nspoof\trow`},
		},
		{
			name:  "upload path",
			value: uploadResult{Files: []uploadedFile{{Path: "file\nspoof\trow", MessageID: 1}}},
			rows:  1, fields: []string{`file\nspoof\trow`},
		},
		{
			name:  "profile",
			value: whoamiResult{ID: 1, Username: "user\nspoof\trow", Phone: "123\nspoof\trow"},
			rows:  1, fields: []string{`user\nspoof\trow`, `123\nspoof\trow`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertTextRows(t, tc.value, tc.rows, tc.tabs, tc.fields...)
		})
	}
}
