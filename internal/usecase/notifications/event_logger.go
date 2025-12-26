package notifications

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/adeithe/go-twitch/irc"
	kickchatwrapper "github.com/johanvandegriff/kick-chat-wrapper"
)

// EventLogger centraliza los logs de eventos de plataformas para facilitar la
// futura ingesta (subs, bits, tips, etc.).
type EventLogger struct {
	now func() time.Time
}

func NewEventLogger() *EventLogger {
	return &EventLogger{
		now: time.Now,
	}
}

// HandleKickMessage registra los mensajes del websocket de Kick que no son chat normal.
func (l *EventLogger) HandleKickMessage(msg kickchatwrapper.ChatMessage) {
	if strings.EqualFold(strings.TrimSpace(msg.Type), "chat") || strings.EqualFold(strings.TrimSpace(msg.Type), "message") {
		return
	}

	l.logPayload("kick", map[string]any{
		"timestamp":   l.now().UTC().Format(time.RFC3339Nano),
		"event_type":  msg.Type,
		"chatroom_id": msg.ChatroomID,
		"payload":     msg,
	})
}

// HandleTwitchUserNotice registra los USERNOTICE que Twitch envía vía IRC (subs, gifts, cheers, etc.).
func (l *EventLogger) HandleTwitchUserNotice(notice irc.UserNotice) {
	payload := map[string]any{
		"timestamp":  l.now().UTC().Format(time.RFC3339Nano),
		"event_type": notice.Type,
		"channel":    notice.IRCMessage.Params,
		"message":    notice.Message,
		"sender":     notice.Sender,
		"raw_tags":   notice.IRCMessage.Tags,
	}
	l.logPayload("twitch", payload)
}

func (l *EventLogger) logPayload(source string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[%s-events] %v", source, payload)
		return
	}
	log.Printf("[%s-events] %s", source, data)
}
