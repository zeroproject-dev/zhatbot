package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	kicksdk "github.com/glichtv/kick-sdk"
	"github.com/joho/godotenv"
)

var (
	client *kicksdk.Client

	// SOLO PARA DESARROLLO
	lastCodeVerifier string
)

// genera un code_verifier aleatorio
func generateCodeVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// genera code_challenge = base64url(SHA256(verifier))
func generateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// STEP 1: iniciar OAuth
func handleStartOAuth(w http.ResponseWriter, r *http.Request) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		http.Error(w, "no pude generar code_verifier", http.StatusInternalServerError)
		return
	}

	lastCodeVerifier = verifier
	challenge := generateCodeChallenge(verifier)

	authURL := client.OAuth().AuthorizationURL(kicksdk.AuthorizationURLInput{
		ResponseType: "code",
		State:        "dev-state",
		Scopes: []kicksdk.OAuthScope{
			kicksdk.ScopeUserRead,
			kicksdk.ScopeChannelRead,
			kicksdk.ScopeChannelWrite,
		},
		CodeChallenge: challenge,
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// STEP 2: callback OAuth
func handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "falta code", http.StatusBadRequest)
		return
	}

	resp, err := client.OAuth().ExchangeCode(
		r.Context(),
		kicksdk.ExchangeCodeInput{
			Code:         code,
			GrantType:    "authorization_code",
			CodeVerifier: lastCodeVerifier,
		},
	)
	if err != nil {
		http.Error(w, "error intercambiando code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	out, err := json.MarshalIndent(resp.Payload, "", "  ")
	if err != nil {
		http.Error(w, "error serializando tokens", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)

	fmt.Println("\n=== COPIA ESTO A TU .env ===")
	fmt.Println(string(out))
}

func main() {
	// ✅ Cargar .env
	if err := godotenv.Load(); err != nil {
		fmt.Println("⚠️  No se pudo cargar .env (se usarán variables del entorno)")
	}

	clientID := os.Getenv("KICK_CLIENT_ID")
	clientSecret := os.Getenv("KICK_CLIENT_SECRET")
	redirectURI := os.Getenv("KICK_REDIRECT_URI")

	if clientID == "" || clientSecret == "" || redirectURI == "" {
		fmt.Println("❌ Faltan KICK_CLIENT_ID, KICK_CLIENT_SECRET o KICK_REDIRECT_URI")
		return
	}

	client = kicksdk.NewClient(
		kicksdk.WithCredentials(kicksdk.Credentials{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURI:  redirectURI,
		}),
	)

	http.HandleFunc("/api/oauth/kick", handleStartOAuth)
	http.HandleFunc("/api/oauth/kick/callback", handleCallback)

	fmt.Println("✅ Abre en el navegador: https://dev.zdev.app/api/oauth/kick")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		fmt.Println("server error:", err)
	}
}
