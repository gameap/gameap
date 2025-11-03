package domain

import "time"

type User struct {
	ID            uint       `db:"id"`             //
	Login         string     `db:"login"`          // maxlen=255
	Email         string     `db:"email"`          // email,maxlen=255
	Password      string     `db:"password"`       //
	RememberToken *string    `db:"remember_token"` //
	Name          *string    `db:"name"`           // maxlen=255
	CreatedAt     *time.Time `db:"created_at"`     //
	UpdatedAt     *time.Time `db:"updated_at"`     //
}
