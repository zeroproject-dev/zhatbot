# Desktop Migration Notes

## Wails integration
- Desktop runtime vive en `/desktop`.
- Ejecutar:
  - `cd web && bun install && bun run build`
  - `cd desktop && wails dev` (requerirá CLI Wails y GOPROXY habilitado).
- `web/src/lib/wailsjs/` es generado por `wails generate` y está excluido del repo.

## Modo detección
- Usa `isWails()` desde `$lib/wails/adapter` para detectar si corre dentro de Wails.
- Eventos: `onHeartbeat(cb)` (solo desktop) → fallback no-op en web.
- Bindings: `ping()` retorna null fuera de Wails.

## Web legacy
- Docker/Nginx continúan para despliegue web tradicional (ver `Dockerfile`, `nginx.conf`).

## Bus de eventos
- El backend publica en un bus interno (`internal/app/events.Bus`) los tópicos:
  - `chat:message`
  - `notifications:event`
  - `app:error`
  - `tts:status`
  - `tts:spoken`
  - `twitch:bot:connected`
  - `twitch:bot:error`
  - (próximamente `stream:status`)
- Desktop re-emite estos eventos mediante `runtime.EventsEmit` para que el frontend se suscriba vía `$lib/wails/adapter`.

## Bridge único en frontend
- `$lib/wails/adapter` es la única puerta a APIs Wails.
- Detecta modo desktop (`isWails`), expone `ping`, `onHeartbeat`, `onChatMessage`, `onTTSStatus`, `onTTSSpoken`.
- En modo web no realiza ninguna llamada (no hay imports directos a `@wails/runtime`).
- Nuevos bindings disponibles desde `callWailsBinding`:
  - Comandos: `ListCommands`, `UpsertCommand`, `DeleteCommand`.
  - Notificaciones: `Notifications_List`, `Notifications_Create`.
  - Categorías: `Category_Search`, `Category_Update`.
  - Stream status: `StreamStatus_List`.
  - Chat: `Chat_SendCommand` (reemplaza WebSocket saliente en desktop).
  - OAuth desktop: `OAuth_Start`, `OAuth_Status`, `OAuth_Logout`. `OAuth_Start` levanta un servidor loopback (`http://127.0.0.1:<puerto>/oauth/callback/<provider>`) y abre el navegador vía `BrowserOpenURL`, usando PKCE sin `client_secret`.
  - Configuración OAuth: `Config_SetTwitchSecret` persiste el secret introducido por el usuario cuando se dispara `oauth:missing-secret`.
  - TTS: `TTS_GetStatus`, `TTS_Enqueue`, `TTS_StopAll`, `TTS_GetSettings`, `TTS_UpdateSettings`.
- Eventos adicionales: `commands:changed`, `tts:status` y `tts:spoken` permiten invalidar el listado local tras cambios.
- OAuth emite `oauth:status` (inicio del flujo), `oauth:missing-secret` (cuando falta el secret de Twitch) y `oauth:complete` (success/error/timeout) para que el frontend refresque las credenciales mediante `OAuth_Status`.
- Conexión Twitch desktop: al detectar tokens válidos, el runtime arranca automáticamente el cliente IRC y publica `twitch:bot:connected` / `twitch:bot:error` para reflejar el estado del bot sin depender de WebSocket legacy.
- Bindings TTS: `TTS_GetStatus`, `TTS_Enqueue`, `TTS_StopAll`, `TTS_GetSettings`, `TTS_UpdateSettings`.
- Guardias: en modo desktop + `ZHATBOT_MODE=development`, el adapter envolvió `fetch` y `WebSocket` globales para loguear cualquier uso inesperado (las llamadas deben migrarse a bindings/eventos).

### Configuración desktop
- En el primer arranque se auto-crea `config.json` en `%APPDATA%/zhatbot/` (Windows) o en `~/.config/zhatbot/` / `~/Library/Application Support/zhatbot/` si no existe.
- Orden de búsqueda:
  1. Variables de entorno (`TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET`, `TWITCH_REDIRECT_URI`, `KICK_CLIENT_ID`, `KICK_REDIRECT_URI`, etc.).
  2. `config.json` (auto-creado, sobrescribible por el usuario).
  3. En `ZHATBOT_MODE=development`, archivos `.env` en el cwd, junto al ejecutable y/o en la carpeta de config.
- El desktop embeddea un `TWITCH_CLIENT_ID` público por defecto; solo es necesario definirlo si se quiere usar otro.
- Twitch exige `client_secret` incluso con PKCE. Ese secreto nunca se embebe: si falta, el backend emite `oauth:missing-secret` y el frontend muestra un modal para capturarlo y almacenarlo mediante `Config_SetTwitchSecret`. El secret se guarda únicamente en `config.json`.
- Al iniciar la app, el runtime lee las credenciales guardadas en SQLite (bot y streamer) y, si están completas, inicia automáticamente el adaptador de Twitch/IRC, publica `twitch:bot:connected` y enruta los chats/comandos al bus. Si el usuario realiza el login durante la sesión, el adaptador se reinicia sin necesidad de cerrar la app. Ante fallos se emite `twitch:bot:error`.
- Ejemplo de `config.json` mínimo para desktop:
```json
{
  "twitch_client_secret": "",
  "twitch_redirect_uri": "http://localhost:17833/oauth/callback/twitch",
  "kick_redirect_uri": "http://localhost:17833/oauth/callback/kick"
}
```
- Registra estos redirect loopback en los paneles de Twitch/Kick (Twitch exige `localhost`, no acepta `127.0.0.1`).
- Los redirect tipo `http://localhost:8080/api/oauth/...` se mantienen solo para el despliegue web legacy.

## TTS runner
- `internal/app/tts/runner` procesa una cola en background, genera audio y emite eventos `tts:status` / `tts:spoken`.
- El runner publica en WS legacy (`domain.TTSEvent`) para mantener compatibilidad web.
- Bindings desktop controlan la cola; `StopAll` cancela el item actual y vacía pendientes.
- El frontend en Wails usa los eventos para mantener UI reactiva; en modo web sigue dependiendo de `/api/tts/*`.
- Twitch exige `client_secret` incluso usando PKCE; guárdalo localmente (`TWITCH_CLIENT_SECRET` o config.json) y no lo publiques.
