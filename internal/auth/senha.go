// Package auth implementa o login único do sistema: uma senha compartilhada,
// guardada como hash bcrypt, e sessões por cookie persistidas no banco.
//
// Não há usuários nem permissões: quem tem a senha entra.
package auth

import "golang.org/x/crypto/bcrypt"

// GerarHash produz o hash bcrypt de uma senha, para colocar em SENHA_HASH.
func GerarHash(senha string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerificarSenha compara a senha digitada com o hash configurado.
// bcrypt faz a comparação em tempo constante.
func VerificarSenha(hash, senha string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(senha)) == nil
}
