package commands

import (
	"context"
	"fmt"
	"strings"

	"zhatBot/internal/domain"
	ttsusecase "zhatBot/internal/usecase/tts"
)

type TTSCommand struct {
	service *ttsusecase.Service
}

func NewTTSCommand(service *ttsusecase.Service) *TTSCommand {
	return &TTSCommand{service: service}
}

func (c *TTSCommand) Name() string {
	return "tts"
}

func (c *TTSCommand) Aliases() []string {
	return []string{}
}

func (c *TTSCommand) SupportsPlatform(domain.Platform) bool {
	return true
}

func (c *TTSCommand) Handle(ctx context.Context, cmdCtx *Context) error {
	if c.service == nil {
		return nil
	}

	if len(cmdCtx.Args) == 0 {
		return c.usage(ctx, cmdCtx)
	}

	first := strings.TrimSpace(cmdCtx.Args[0])
	lower := strings.ToLower(first)

	switch {
	case lower == "voice:list":
		return c.handleList(ctx, cmdCtx)
	case strings.HasPrefix(lower, "voice:"):
		code := strings.TrimSpace(first[len("voice:"):])
		if code == "" && len(cmdCtx.Args) > 1 {
			code = strings.TrimSpace(cmdCtx.Args[1])
		}
		return c.handleSetVoice(ctx, cmdCtx, code)
	default:
		text := strings.Join(cmdCtx.Args, " ")
		return c.handleRequest(ctx, cmdCtx, text)
	}
}

func (c *TTSCommand) handleList(ctx context.Context, cmdCtx *Context) error {
	if !cmdCtx.Message.IsPlatformAdmin {
		return nil
	}
	voices := c.service.ListVoices()
	parts := make([]string, 0, len(voices))
	for _, voice := range voices {
		parts = append(parts, fmt.Sprintf("%s (%s)", voice.Code, voice.Label))
	}
	return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
		"Voces disponibles: "+strings.Join(parts, ", "))
}

func (c *TTSCommand) handleSetVoice(ctx context.Context, cmdCtx *Context, code string) error {
	if !cmdCtx.Message.IsPlatformAdmin {
		return nil
	}
	if strings.TrimSpace(code) == "" {
		return c.usage(ctx, cmdCtx)
	}
	voice, err := c.service.SetVoice(ctx, code)
	if err != nil {
		return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
			fmt.Sprintf("⚠️ %v", err))
	}
	return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
		fmt.Sprintf("✅ Voz TTS establecida en %s (%s)", voice.Code, voice.Label))
}

func (c *TTSCommand) handleRequest(ctx context.Context, cmdCtx *Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return c.usage(ctx, cmdCtx)
	}
	if err := c.service.RequestSpeech(ctx, text, cmdCtx.Message.Username, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID); err != nil {
		return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
			fmt.Sprintf("⚠️ %v", err))
	}
	voice := c.service.CurrentVoice(ctx)
	return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
		fmt.Sprintf("🔊 Enviado a reproducción (%s)", voice.Code))
}

func (c *TTSCommand) usage(ctx context.Context, cmdCtx *Context) error {
	return cmdCtx.Out.SendMessage(ctx, cmdCtx.Message.Platform, cmdCtx.Message.ChannelID,
		"Uso: !tts voice:list | !tts voice:<id> | !tts <texto>")
}
