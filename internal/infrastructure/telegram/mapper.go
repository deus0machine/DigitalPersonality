package telegram

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
)

// rawMessageSnapshot is stored in messages.raw_data.
// Captures every personality-relevant field without gotd/td types at the storage boundary.
type rawMessageSnapshot struct {
	TelegramID    int             `json:"id"`
	Date          int             `json:"date"`
	Out           bool            `json:"out,omitempty"`
	FromID        int64           `json:"from_id,omitempty"`
	PeerType      string          `json:"peer_type"`
	ReplyToID     int             `json:"reply_to_id,omitempty"`
	MediaType     string          `json:"media_type,omitempty"`
	Views         int             `json:"views,omitempty"`
	Forwards      int             `json:"forwards,omitempty"`
	IsForwarded   bool            `json:"is_forwarded,omitempty"`
	ForwardFromID int64           `json:"forward_from_id,omitempty"`
	ForwardDate   int             `json:"forward_date,omitempty"`
	EditDate      int             `json:"edit_date,omitempty"`
	Entities      []rawEntity     `json:"entities,omitempty"`
	Reactions     []rawReaction   `json:"reactions,omitempty"`
	StickerMeta   *rawStickerMeta `json:"sticker,omitempty"`
}

type rawEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

type rawReaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

type rawStickerMeta struct {
	SetName  string `json:"set_name,omitempty"`
	Emoticon string `json:"emoticon,omitempty"`
}

// mapMessage converts a gotd/td Message into a domain-neutral IncomingMessage.
// selfID is required to infer sender identity for private chats.
func mapMessage(msg *tg.Message, selfID int64) (port.IncomingMessage, bool) {
	if msg == nil {
		return port.IncomingMessage{}, false
	}

	chatID, peerType := peerIDAndType(msg.PeerID)
	senderID := resolveSenderID(msg, chatID, selfID)
	replyToID := resolveReplyToID(msg)
	fwdFrom, hasFwd := msg.GetFwdFrom()
	isForwarded, forwardFromID, forwardDate := resolveForward(fwdFrom, hasFwd)
	editDate := resolveEditDate(msg.EditDate)

	entities := mapEntities(msg.Entities)
	reactions := mapReactions(msg.Reactions)
	mediaKind, sticker := resolveMedia(msg.Media)

	raw, _ := json.Marshal(rawMessageSnapshot{
		TelegramID:    msg.ID,
		Date:          msg.Date,
		Out:           msg.Out,
		FromID:        senderID,
		PeerType:      peerType,
		ReplyToID:     int(replyToID),
		MediaType:     string(mediaKind),
		Views:         msg.Views,
		Forwards:      msg.Forwards,
		IsForwarded:   isForwarded,
		ForwardFromID: forwardFromID,
		ForwardDate:   int(forwardDate.Unix()),
		EditDate:      int(editDate.Unix()),
		Entities:      toRawEntities(entities),
		Reactions:     toRawReactions(reactions),
		StickerMeta:   toRawSticker(sticker),
	})

	return port.IncomingMessage{
		TelegramID:    int64(msg.ID),
		ChatID:        chatID,
		SenderID:      senderID,
		ReplyToID:     replyToID,
		Text:          msg.Message,
		RawData:       raw,
		SentAt:        time.Unix(int64(msg.Date), 0).UTC(),
		IsOutgoing:    msg.Out,
		IsForwarded:   isForwarded,
		ForwardFromID: forwardFromID,
		ForwardDate:   forwardDate,
		EditDate:      editDate,
		Entities:      entities,
		Reactions:     reactions,
		MediaKind:     string(mediaKind),
		Sticker:       sticker,
	}, true
}

// mapDialogInfo builds a domain-neutral DialogInfo from gotd dialog components.
func mapDialogInfo(
	dialog *tg.Dialog,
	users map[int64]*tg.User,
	chats map[int64]*tg.Chat,
	channels map[int64]*tg.Channel,
) (port.DialogInfo, bool) {
	switch peer := dialog.Peer.(type) {
	case *tg.PeerUser:
		u, ok := users[peer.UserID]
		if !ok {
			return port.DialogInfo{}, false
		}
		// Saved Messages: self-dialog for notes, links, reminders.
		if u.Self {
			return port.DialogInfo{
				ID:         u.ID,
				Type:       entity.ChatTypeSavedMessages,
				Title:      "Saved Messages",
				AccessHash: u.AccessHash,
			}, true
		}
		// Bots are included with IsBot=true; scorer assigns medium relevance.
		name := strings.TrimSpace(u.FirstName + " " + u.LastName)
		if name == "" {
			name = u.Username // bots often lack a first/last name
		}
		return port.DialogInfo{
			ID:         u.ID,
			Type:       entity.ChatTypePrivate,
			Title:      name,
			Username:   u.Username,
			AccessHash: u.AccessHash,
			IsBot:      u.Bot,
		}, true

	case *tg.PeerChat:
		c, ok := chats[peer.ChatID]
		if !ok {
			return port.DialogInfo{}, false
		}
		_, hasAdminRights := c.GetAdminRights()
		return port.DialogInfo{
			ID:        c.ID,
			Type:      entity.ChatTypeGroup,
			Title:     c.Title,
			IsCreator: c.Creator,
			IsAdmin:   hasAdminRights,
		}, true

	case *tg.PeerChannel:
		ch, ok := channels[peer.ChannelID]
		if !ok {
			return port.DialogInfo{}, false
		}
		_, hasAdminRights := ch.GetAdminRights()
		chatType := entity.ChatTypeChannel
		if ch.Megagroup {
			chatType = entity.ChatTypeSupergroup
		}
		return port.DialogInfo{
			ID:          ch.ID,
			Type:        chatType,
			Title:       ch.Title,
			Username:    ch.Username,
			AccessHash:  ch.AccessHash,
			IsCreator:   ch.Creator,
			IsAdmin:     hasAdminRights,
			IsBroadcast: ch.Broadcast,
		}, true

	default:
		return port.DialogInfo{}, false
	}
}

// mapUserInfo converts a tg.User into a domain-neutral UserInfo.
func mapUserInfo(u *tg.User) *port.UserInfo {
	return &port.UserInfo{
		ID:        u.ID,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
		IsSelf:    u.Self,
	}
}

// mapParticipants extracts real users from a history page's user list.
// tg.UserEmpty (deleted/inaccessible accounts) are skipped — EnsureExists
// handles any remaining FK gaps on the message insert path.
func mapParticipants(rawUsers []tg.UserClass) []port.UserInfo {
	if len(rawUsers) == 0 {
		return nil
	}
	out := make([]port.UserInfo, 0, len(rawUsers))
	for _, u := range rawUsers {
		user, ok := u.(*tg.User)
		if !ok || user.ID == 0 {
			continue // tg.UserEmpty or zero-ID — skip
		}
		out = append(out, *mapUserInfo(user))
	}
	return out
}

// buildDialogLookups builds fast lookup maps from the dialog response slices.
func buildDialogLookups(
	rawUsers []tg.UserClass,
	rawChats []tg.ChatClass,
) (users map[int64]*tg.User, chats map[int64]*tg.Chat, channels map[int64]*tg.Channel) {
	users = make(map[int64]*tg.User, len(rawUsers))
	chats = make(map[int64]*tg.Chat)
	channels = make(map[int64]*tg.Channel)

	for _, u := range rawUsers {
		if user, ok := u.(*tg.User); ok {
			users[user.ID] = user
		}
	}
	for _, c := range rawChats {
		switch v := c.(type) {
		case *tg.Chat:
			chats[v.ID] = v
		case *tg.Channel:
			channels[v.ID] = v
		}
	}
	return
}

// inputPeerFromDialog reconstructs the InputPeer needed for history requests.
func inputPeerFromDialog(d port.DialogInfo) tg.InputPeerClass {
	switch d.Type {
	case entity.ChatTypeSavedMessages:
		return &tg.InputPeerSelf{}
	case entity.ChatTypePrivate:
		return &tg.InputPeerUser{UserID: d.ID, AccessHash: d.AccessHash}
	case entity.ChatTypeGroup:
		return &tg.InputPeerChat{ChatID: d.ID}
	case entity.ChatTypeChannel, entity.ChatTypeSupergroup:
		return &tg.InputPeerChannel{ChannelID: d.ID, AccessHash: d.AccessHash}
	default:
		return &tg.InputPeerEmpty{}
	}
}

// ─── Entity mapping ───────────────────────────────────────────────────────────

func mapEntities(raw []tg.MessageEntityClass) []port.MessageEntity {
	if len(raw) == 0 {
		return nil
	}
	result := make([]port.MessageEntity, 0, len(raw))
	for _, e := range raw {
		result = append(result, mapEntity(e))
	}
	return result
}

func mapEntity(e tg.MessageEntityClass) port.MessageEntity {
	base := port.MessageEntity{}
	switch v := e.(type) {
	case *tg.MessageEntityBold:
		base.Type, base.Offset, base.Length = "bold", v.Offset, v.Length
	case *tg.MessageEntityItalic:
		base.Type, base.Offset, base.Length = "italic", v.Offset, v.Length
	case *tg.MessageEntityCode:
		base.Type, base.Offset, base.Length = "code", v.Offset, v.Length
	case *tg.MessageEntityPre:
		base.Type, base.Offset, base.Length = "pre", v.Offset, v.Length
	case *tg.MessageEntityURL:
		base.Type, base.Offset, base.Length = "url", v.Offset, v.Length
	case *tg.MessageEntityTextURL:
		base.Type, base.Offset, base.Length, base.URL = "text_url", v.Offset, v.Length, v.URL
	case *tg.MessageEntityMention:
		base.Type, base.Offset, base.Length = "mention", v.Offset, v.Length
	case *tg.MessageEntityHashtag:
		base.Type, base.Offset, base.Length = "hashtag", v.Offset, v.Length
	case *tg.MessageEntityCashtag:
		base.Type, base.Offset, base.Length = "cashtag", v.Offset, v.Length
	case *tg.MessageEntityStrike:
		base.Type, base.Offset, base.Length = "strikethrough", v.Offset, v.Length
	case *tg.MessageEntityUnderline:
		base.Type, base.Offset, base.Length = "underline", v.Offset, v.Length
	case *tg.MessageEntitySpoiler:
		base.Type, base.Offset, base.Length = "spoiler", v.Offset, v.Length
	case *tg.MessageEntityBlockquote:
		base.Type, base.Offset, base.Length = "blockquote", v.Offset, v.Length
	case *tg.MessageEntityCustomEmoji:
		base.Type, base.Offset, base.Length = "custom_emoji", v.Offset, v.Length
	default:
		base.Type = "unknown"
	}
	return base
}

// ─── Reaction mapping ─────────────────────────────────────────────────────────

// mapReactions takes tg.MessageReactions by value (gotd/td uses a value type for it).
func mapReactions(raw tg.MessageReactions) []port.ReactionStat {
	if len(raw.Results) == 0 {
		return nil
	}
	result := make([]port.ReactionStat, 0, len(raw.Results))
	for _, rc := range raw.Results {
		emoji := reactionEmoji(rc.Reaction)
		if emoji == "" {
			continue
		}
		result = append(result, port.ReactionStat{Emoji: emoji, Count: rc.Count})
	}
	return result
}

func reactionEmoji(r tg.ReactionClass) string {
	switch v := r.(type) {
	case *tg.ReactionEmoji:
		return v.Emoticon
	case *tg.ReactionCustomEmoji:
		return "custom:" + string(rune(v.DocumentID)) // best-effort for custom emoji
	default:
		return ""
	}
}

// ─── Media / sticker mapping ──────────────────────────────────────────────────

func resolveMedia(media tg.MessageMediaClass) (entity.MediaKind, *port.StickerInfo) {
	if media == nil {
		return entity.MediaKindNone, nil
	}
	switch v := media.(type) {
	case *tg.MessageMediaPhoto:
		return entity.MediaKindPhoto, nil
	case *tg.MessageMediaDocument:
		return resolveDocument(v)
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive:
		return entity.MediaKindGeo, nil
	case *tg.MessageMediaContact:
		return entity.MediaKindContact, nil
	case *tg.MessageMediaPoll:
		return entity.MediaKindPoll, nil
	default:
		return entity.MediaKindDocument, nil
	}
}

func resolveDocument(v *tg.MessageMediaDocument) (entity.MediaKind, *port.StickerInfo) {
	doc, ok := v.Document.(*tg.Document)
	if !ok {
		return entity.MediaKindDocument, nil
	}
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeSticker:
			sticker := &port.StickerInfo{Emoticon: a.Alt}
			if set, ok := a.Stickerset.(*tg.InputStickerSetID); ok {
				_ = set // set ID available; name requires a separate API call
			}
			return entity.MediaKindSticker, sticker
		case *tg.DocumentAttributeAudio:
			if a.Voice {
				return entity.MediaKindVoice, nil
			}
		case *tg.DocumentAttributeVideo:
			if a.RoundMessage {
				return entity.MediaKindRound, nil
			}
			return entity.MediaKindVideo, nil
		}
	}
	return entity.MediaKindDocument, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func peerIDAndType(peer tg.PeerClass) (int64, string) {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID, "user"
	case *tg.PeerChat:
		return p.ChatID, "chat"
	case *tg.PeerChannel:
		return p.ChannelID, "channel"
	default:
		return 0, "unknown"
	}
}

func resolveSenderID(msg *tg.Message, chatID, selfID int64) int64 {
	if msg.FromID != nil {
		if from, ok := msg.FromID.(*tg.PeerUser); ok {
			return from.UserID
		}
	}
	if _, isUser := msg.PeerID.(*tg.PeerUser); isUser {
		if msg.Out {
			return selfID
		}
		return chatID
	}
	return 0
}

func resolveReplyToID(msg *tg.Message) int64 {
	if msg.ReplyTo == nil {
		return 0
	}
	if r, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok {
		return int64(r.ReplyToMsgID)
	}
	return 0
}

func toRawEntities(es []port.MessageEntity) []rawEntity {
	if len(es) == 0 {
		return nil
	}
	out := make([]rawEntity, len(es))
	for i, e := range es {
		out[i] = rawEntity{Type: e.Type, Offset: e.Offset, Length: e.Length, URL: e.URL}
	}
	return out
}

func toRawReactions(rs []port.ReactionStat) []rawReaction {
	if len(rs) == 0 {
		return nil
	}
	out := make([]rawReaction, len(rs))
	for i, r := range rs {
		out[i] = rawReaction{Emoji: r.Emoji, Count: r.Count}
	}
	return out
}

func toRawSticker(s *port.StickerInfo) *rawStickerMeta {
	if s == nil {
		return nil
	}
	return &rawStickerMeta{SetName: s.SetName, Emoticon: s.Emoticon}
}

// resolveForward extracts forward metadata from a MessageFwdHeader.
// hasFwd must come from msg.GetFwdFrom() — false means not a forward.
func resolveForward(fwd tg.MessageFwdHeader, hasFwd bool) (isForwarded bool, fromID int64, date time.Time) {
	if !hasFwd {
		return false, 0, time.Time{}
	}
	if from, ok := fwd.GetFromID(); ok {
		if peer, ok := from.(*tg.PeerUser); ok {
			fromID = peer.UserID
		}
		// PeerChannel/PeerChat forwards: fromID stays 0 (anonymous/channel source)
	}
	return true, fromID, time.Unix(int64(fwd.Date), 0).UTC()
}

// resolveEditDate converts a Telegram unix edit timestamp to time.Time.
// Returns zero time when the message has never been edited (editDate == 0).
func resolveEditDate(editDate int) time.Time {
	if editDate == 0 {
		return time.Time{}
	}
	return time.Unix(int64(editDate), 0).UTC()
}
