package auth

import (
	"log/slog"
	"net/http"
	"net/url"
)

// NomeCookie é o nome do cookie de sessão.
const NomeCookie = "sessao"

// RequerLogin bloqueia quem não tem sessão válida, mandando para /login.
//
// Para requisições do HTMX (parciais), um redirect normal trocaria só um
// pedaço da página pela tela de login; por isso usamos o cabeçalho HX-Redirect,
// que faz o navegador inteiro navegar.
func RequerLogin(sessoes *Sessoes, proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := TokenDoCookie(r)
		valida, err := sessoes.Valida(r.Context(), token)
		if err != nil {
			slog.Error("validar sessão", "erro", err)
			http.Error(w, "erro interno", http.StatusInternalServerError)
			return
		}
		if !valida {
			destino := "/login?next=" + url.QueryEscape(r.URL.RequestURI())
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", destino)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, destino, http.StatusSeeOther)
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

// TokenDoCookie devolve o token da sessão, ou "" se não houver cookie.
func TokenDoCookie(r *http.Request) string {
	c, err := r.Cookie(NomeCookie)
	if err != nil {
		return ""
	}
	return c.Value
}
