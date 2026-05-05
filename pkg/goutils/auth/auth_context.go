package auth

import "golang.org/x/oauth2"

// provides an interface for saving and retrieving an oauth token
type AuthToken interface {
	SetToken(token *oauth2.Token)
	GetToken() *oauth2.Token
}
