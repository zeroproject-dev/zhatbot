package commands

import "zhatBot/internal/domain"

// CommandDescriptor expone metadatos de cada comando interno para mostrarlos en UI.
type CommandDescriptor struct {
	Name        string
	Aliases     []string
	Platforms   []domain.Platform
	Description string
	Usage       string
	Permissions []domain.CommandAccessRole
}

// BuiltinCommandCatalog describe los comandos que vienen incluidos en el bot.
func BuiltinCommandCatalog() []CommandDescriptor {
	return []CommandDescriptor{
		{
			Name:        "ping",
			Platforms:   []domain.Platform{domain.PlatformTwitch, domain.PlatformKick},
			Description: "Responde con «pong» para probar la conexión del bot.",
			Usage:       "!ping",
			Permissions: []domain.CommandAccessRole{domain.CommandAccessEveryone},
		},
		{
			Name:        "command",
			Description: "Administra los comandos personalizados (crear, editar o eliminar).",
			Usage:       "!command <nombre> [aliases:a,b] [platforms:twitch] [permissions:everyone] <respuesta>",
			Permissions: []domain.CommandAccessRole{domain.CommandAccessOwner},
		},
		{
			Name:        "title",
			Description: "Actualiza el título del stream en las plataformas conectadas.",
			Usage:       "!title <nuevo título>",
			Platforms:   []domain.Platform{domain.PlatformTwitch, domain.PlatformKick},
			Permissions: []domain.CommandAccessRole{domain.CommandAccessOwner},
		},
		{
			Name:        "tts",
			Description: "Solicita lecturas TTS o gestiona voces/start/stop desde el chat.",
			Usage:       "!tts <texto> | !tts voice:list | !tts voice:start|stop",
			Permissions: []domain.CommandAccessRole{domain.CommandAccessEveryone},
		},
	}
}
