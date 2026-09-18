package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/Gustavo-Resende/autoReboque/internal/auth"
)

func (s *Servidor) loginForm(w http.ResponseWriter, r *http.Request) {
	// Quem já está logado não precisa ver a tela de login.
	if valida, _ := s.sessoes.Valida(r.Context(), auth.TokenDoCookie(r)); valida {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d := s.base(w, r, "Entrar", "")
	d["Logado"] = false
	d["Next"] = destinoSeguro(r.URL.Query().Get("next"))
	if err := s.tpl.Renderizar(w, http.StatusOK, "pages/login.html", d); err != nil {
		s.erroInterno(w, r, err)
	}
}

func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.erro(w, r, http.StatusBadRequest, "Formulário inválido.")
		return
	}
	senha := r.PostForm.Get("senha")
	next := destinoSeguro(r.PostForm.Get("next"))

	if !auth.VerificarSenha(s.cfg.SenhaHash, senha) {
		// Um pequeno atraso encarece tentativas em sequência sem atrapalhar ninguém.
		time.Sleep(500 * time.Millisecond)
		d := s.base(w, r, "Entrar", "")
		d["Logado"] = false
		d["Next"] = next
		d["Erro"] = "Senha incorreta."
		if err := s.tpl.Renderizar(w, http.StatusUnauthorized, "pages/login.html", d); err != nil {
			s.erroInterno(w, r, err)
		}
		return
	}

	token, err := s.sessoes.Criar(r.Context())
	if err != nil {
		s.erroInterno(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.NomeCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.sessoes.Duracao().Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Servidor) logout(w http.ResponseWriter, r *http.Request) {
	if token := auth.TokenDoCookie(r); token != "" {
		_ = s.sessoes.Encerrar(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.NomeCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// destinoSeguro só aceita caminhos locais como destino pós-login,
// para o parâmetro "next" não servir de redirecionamento para outro site.
func destinoSeguro(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}
