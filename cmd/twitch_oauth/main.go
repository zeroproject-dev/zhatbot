package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const (
	twitchAuthorizeURL = "https://id.twitch.tv/oauth2/authorize"
	twitchTokenURL     = "https://id.twitch.tv/oauth2/token"
)

// SOLO PARA DESARROLLO
var (
	lastCodeVerifier string
	lastOAuthRole    string // "bot" o "streamer"
)

// =========================
// PKCE helpers
// =========================

func generateCodeVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func generateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// =========================
// STEP 1: iniciar OAuth
// =========================

func handleStartOAuth(role string, scopes []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		verifier, err := generateCodeVerifier()
		if err != nil {
			http.Error(w, "no pude generar code_verifier", http.StatusInternalServerError)
			return
		}

		lastCodeVerifier = verifier
		lastOAuthRole = role
		challenge := generateCodeChallenge(verifier)

		q := url.Values{}
		q.Set("client_id", os.Getenv("TWITCH_CLIENT_ID"))
		q.Set("redirect_uri", os.Getenv("TWITCH_REDIRECT_URI"))
		q.Set("response_type", "code")
		q.Set("scope", strings.Join(scopes, " "))
		q.Set("state", role)
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")

		authURL := twitchAuthorizeURL + "?" + q.Encode()
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

// =========================
// STEP 2: callback OAuth
// =========================

func handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		http.Error(w, "falta code", http.StatusBadRequest)
		return
	}

	if state != lastOAuthRole {
		http.Error(w, "state inválido", http.StatusBadRequest)
		return
	}

	data := url.Values{}
	data.Set("client_id", os.Getenv("TWITCH_CLIENT_ID"))
	data.Set("client_secret", os.Getenv("TWITCH_CLIENT_SECRET"))
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", os.Getenv("TWITCH_REDIRECT_URI"))
	data.Set("code_verifier", lastCodeVerifier)

	req, err := http.NewRequest("POST", twitchTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, "error creando request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "error llamando a token endpoint", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "error parseando respuesta", http.StatusInternalServerError)
		return
	}

	out, _ := json.MarshalIndent(payload, "", "  ")

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)

	fmt.Println("\n==============================")
	fmt.Printf("✅ TOKENS PARA: %s\n", strings.ToUpper(state))
	fmt.Println(string(out))
	fmt.Println("==============================")
}

// =========================
// MAIN
// =========================

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("⚠️  No se pudo cargar .env")
	}

	required := []string{
		"TWITCH_CLIENT_ID",
		"TWITCH_CLIENT_SECRET",
		"TWITCH_REDIRECT_URI",
	}

	for _, k := range required {
		if os.Getenv(k) == "" {
			fmt.Printf("❌ Falta %s en .env\n", k)
			return
		}
	}

	// STREAMER
	http.HandleFunc(
		"/api/oauth/twitch/streamer",
		handleStartOAuth(
			"streamer",
			[]string{
				"channel:manage:broadcast",
			},
		),
	)

	// BOT
	http.HandleFunc(
		"/api/oauth/twitch/bot",
		handleStartOAuth(
			"bot",
			[]string{
				"chat:read",
				"chat:edit",
			},
		),
	)

	// CALLBACK
	http.HandleFunc("/api/oauth/twitch/callback", handleCallback)

	fmt.Println("✅ Twitch OAuth listo")
	fmt.Println("➡ Streamer: https://dev.zdev.app/api/oauth/twitch/streamer")
	fmt.Println("➡ Bot:      https://dev.zdev.app/api/oauth/twitch/bot")

	if err := http.ListenAndServe(":3000", nil); err != nil {
		fmt.Println("server error:", err)
	}
}
