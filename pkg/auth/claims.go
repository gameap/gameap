package auth

type Claims interface {
	GetSubject() (string, error)
}
