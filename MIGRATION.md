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
  - (próximamente `stream:status`)
- Desktop re-emite estos eventos mediante `runtime.EventsEmit` para que el frontend se suscriba vía `$lib/wails/adapter`.

## Bridge único en frontend
- `$lib/wails/adapter` es la única puerta a APIs Wails.
- Detecta modo desktop (`isWails`), expone `ping`, `onHeartbeat`, `onChatMessage`, `onTTSStatus`, `onTTSSpoken`.
- En modo web no realiza ninguna llamada (no hay imports directos a `@wails/runtime`).
- Nuevos bindings: `ListCommands`, `UpsertCommand`, `DeleteCommand` (desktop) se consumen vía `callWailsBinding`.
- Eventos adicionales: `commands:changed`, `tts:status` y `tts:spoken` permiten invalidar el listado local tras cambios.
- Bindings TTS: `TTS_GetStatus`, `TTS_Enqueue`, `TTS_StopAll`, `TTS_GetSettings`, `TTS_UpdateSettings`.

## TTS runner
- `internal/app/tts/runner` procesa una cola en background, genera audio y emite eventos `tts:status` / `tts:spoken`.
- El runner publica en WS legacy (`domain.TTSEvent`) para mantener compatibilidad web.
- Bindings desktop controlan la cola; `StopAll` cancela el item actual y vacía pendientes.
- El frontend en Wails usa los eventos para mantener UI reactiva; en modo web sigue dependiendo de `/api/tts/*`.
