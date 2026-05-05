package mycsnode

type AuthRequest struct {
	AuthReqIDKey string `json:"authReqIDKey"`
	AuthReqKey   string `json:"authReqKey"`
}
type AuthReqKey struct {
	RefID   string `json:"refID"`
	ECDHKey string `json:"ecdhKey"`
	Nonce   int64  `json:"nonce"`
}
type AuthResponse struct {
	AuthRespIDKey string `json:"authRespIDKey"`
	AuthRespKey   string `json:"authRespKey"`
}
type AuthRespKey struct {
	NodeECDHKey string `json:"nodeECDHKey"`
	Nonce       int64  `json:"nonce"`
	TimeoutAt   int64  `json:"timeoutAt"`
	RefName     string `json:"refName"`
}
